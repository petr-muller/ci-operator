package steps

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"path/filepath"

	coreapi "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"

	imageclientset "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"

	vmapi "github.com/openshift/ci-vm-operator/pkg/apis/virtualmachines/v1alpha1"

	"github.com/openshift/ci-operator/pkg/api"
)

type PodStepConfiguration struct {
	As                 string
	From               api.ImageStreamTagReference
	Commands           string
	ArtifactDir        string
	ServiceAccountName string
}

type VMStepConfiguration struct {
	As string
}

type vmStep struct {
	name     string
	config   VMStepConfiguration
	vmClient VMClient
	jobSpec  *api.JobSpec
}

type podStep struct {
	name        string
	config      PodStepConfiguration
	resources   api.ResourceConfiguration
	podClient   PodClient
	istClient   imageclientset.ImageStreamTagsGetter
	artifactDir string
	jobSpec     *api.JobSpec
}

func (s *podStep) Inputs(ctx context.Context, dry bool) (api.InputDefinition, error) {
	return nil, nil
}

func (s *vmStep) Inputs(ctx context.Context, dry bool) (api.InputDefinition, error) {
	return nil, nil
}

func (s *podStep) Run(ctx context.Context, dry bool) error {
	log.Printf("Executing %s %s", s.name, s.config.As)

	containerResources, err := resourcesFor(s.resources.RequirementsForStep(s.config.As))
	if err != nil {
		return fmt.Errorf("unable to calculate %s pod resources for %s: %s", s.name, s.config.As, err)
	}

	if len(s.config.From.Namespace) > 0 {
		return fmt.Errorf("pod step does not supported an image stream tag reference outside the namespace")
	}
	image := fmt.Sprintf("%s:%s", s.config.From.Name, s.config.From.Tag)

	pod := &coreapi.Pod{
		ObjectMeta: meta.ObjectMeta{
			Name: s.config.As,
			Labels: trimLabels(map[string]string{
				PersistsLabel:    "false",
				JobLabel:         s.jobSpec.Job,
				BuildIdLabel:     s.jobSpec.BuildId,
				ProwJobIdLabel:   s.jobSpec.ProwJobID,
				CreatedByCILabel: "true",
			}),
			Annotations: map[string]string{
				JobSpecAnnotation: s.jobSpec.RawSpec(),
			},
		},
		Spec: coreapi.PodSpec{
			ServiceAccountName: s.config.ServiceAccountName,
			RestartPolicy:      coreapi.RestartPolicyNever,
			Containers: []coreapi.Container{
				{
					Name:                     s.name,
					Image:                    image,
					Command:                  []string{"/bin/sh", "-c", "#!/bin/sh\nset -eu\n" + s.config.Commands},
					Resources:                containerResources,
					TerminationMessagePolicy: coreapi.TerminationMessageFallbackToLogsOnError,
				},
			},
		},
	}

	// when the test container terminates and artifact directory has been set, grab everything under the directory
	var notifier ContainerNotifier = NopNotifier
	if s.gatherArtifacts() {
		artifacts := NewArtifactWorker(s.podClient, filepath.Join(s.artifactDir, s.config.As), s.jobSpec.Namespace)
		pod.Spec.Containers[0].VolumeMounts = append(pod.Spec.Containers[0].VolumeMounts, coreapi.VolumeMount{
			Name:      "artifacts",
			MountPath: s.config.ArtifactDir,
		})
		addArtifactsContainer(pod, s.config.ArtifactDir)
		artifacts.CollectFromPod(pod.Name, []string{s.name}, nil)
		notifier = artifacts
	}

	if owner := s.jobSpec.Owner(); owner != nil {
		pod.OwnerReferences = append(pod.OwnerReferences, *owner)
	}

	if dry {
		j, _ := json.MarshalIndent(pod, "", "  ")
		log.Printf("pod:\n%s", j)
		return nil
	}

	go func() {
		<-ctx.Done()
		notifier.Cancel()
		log.Printf("cleanup: Deleting %s pod %s", s.name, s.config.As)
		if err := s.podClient.Pods(s.jobSpec.Namespace).Delete(s.config.As, nil); err != nil && !errors.IsNotFound(err) {
			log.Printf("error: Could not delete %s pod: %v", s.name, err)
		}
	}()

	pod, err = createOrRestartPod(s.podClient.Pods(s.jobSpec.Namespace), pod)
	if err != nil {
		return fmt.Errorf("failed to create or restart %s pod: %v", s.name, err)
	}

	if err := waitForPodCompletion(s.podClient.Pods(s.jobSpec.Namespace), pod.Name, notifier); err != nil {
		return fmt.Errorf("failed to wait for %s pod to complete: %v", s.name, err)
	}

	return nil
}

func (s *vmStep) Run(ctx context.Context, dry bool) error {
	log.Printf("Executing %s %s", s.name, s.config.As)

	machineSpec := &vmapi.VirtualMachine{
		ObjectMeta: meta.ObjectMeta{
			Name: s.config.As,
		},
		Spec: vmapi.VirtualMachineSpec{
			MachineType: vmapi.VirtualMachineTypeStandard1,
			BootDisk: vmapi.VirtualMachineBootDiskSpec{
				ImageFamily: "compute/v1/projects/centos-cloud/global/images/family/centos-6",
				VirtualMachineDiskSpec: vmapi.VirtualMachineDiskSpec{
					SizeGB: 25,
					Type:   vmapi.VirtualMachineDiskTypePersistentStandard,
				},
			},
		},
	}

	fmt.Printf("Created a VM instance: %v", machineSpec)

	if _, err := s.vmClient.VirtualMachines(s.jobSpec.Namespace).Create(machineSpec); err != nil {
		return err
	}

	return nil
}

func (s *podStep) gatherArtifacts() bool {
	return len(s.config.ArtifactDir) > 0 && len(s.artifactDir) > 0
}

func (s *podStep) Done() (bool, error) {
	ready, err := isPodCompleted(s.podClient.Pods(s.jobSpec.Namespace), s.config.As)
	if err != nil {
		return false, fmt.Errorf("failed to determine if %s pod was completed: %v", s.name, err)
	}
	if !ready {
		return false, nil
	}
	return true, nil
}

func (s *vmStep) Done() (bool, error) {
	return false, nil
}

func (s *podStep) Requires() []api.StepLink {
	if s.config.From.Name == api.PipelineImageStream {
		return []api.StepLink{api.InternalImageLink(api.PipelineImageStreamTagReference(s.config.From.Tag))}
	}
	return []api.StepLink{api.ImagesReadyLink()}
}

func (s *vmStep) Requires() []api.StepLink {
	return []api.StepLink{api.ImagesReadyLink()}

}

func (s *podStep) Creates() []api.StepLink {
	return []api.StepLink{}
}

func (s *vmStep) Creates() []api.StepLink {
	return []api.StepLink{}
}

func (s *podStep) Provides() (api.ParameterMap, api.StepLink) {
	return nil, nil
}

func (s *vmStep) Provides() (api.ParameterMap, api.StepLink) {
	return nil, nil
}

func (s *podStep) Name() string { return s.config.As }

func (s *vmStep) Name() string {
	fmt.Printf("Got here: '%v'\n", s.config)
	return s.config.As
}

func (s *podStep) Description() string {
	return fmt.Sprintf("Run the tests for %s in a pod and wait for success or failure", s.config.As)
}

func (s *vmStep) Description() string {
	return fmt.Sprintf("Run a VM-based test '%s' and wait for success or failure", s.name)
}

func VMTestStep(config api.TestStepConfiguration, vmClient VMClient, jobSpec *api.JobSpec) api.Step {
	if config.VirtualMachineTestConfiguration != nil {
		fmt.Printf("Got also here: %v\n", config.As)

		return VMStep(
			"VM test",
			VMStepConfiguration{
				As: config.As,
			},
			vmClient,
			jobSpec,
		)
	}

	panic(fmt.Sprintf("Unknown TestStep type: %v", config))
}

func TestStep(config api.TestStepConfiguration, resources api.ResourceConfiguration, podClient PodClient, artifactDir string, jobSpec *api.JobSpec) api.Step {
	if config.ContainerTestConfiguration != nil {
		return PodStep(
			"test",
			PodStepConfiguration{
				As:          config.As,
				From:        api.ImageStreamTagReference{Name: api.PipelineImageStream, Tag: string(config.ContainerTestConfiguration.From)},
				Commands:    config.Commands,
				ArtifactDir: config.ArtifactDir,
			},
			resources,
			podClient,
			artifactDir,
			jobSpec,
		)
	}
	panic(fmt.Sprintf("Unknown TestStep type: %v", config))
}

func VMStep(name string, config VMStepConfiguration, vmClient VMClient, jobSpec *api.JobSpec) api.Step {
	return &vmStep{
		name:     name,
		config:   config,
		vmClient: vmClient,
		jobSpec:  jobSpec,
	}
}

func PodStep(name string, config PodStepConfiguration, resources api.ResourceConfiguration, podClient PodClient, artifactDir string, jobSpec *api.JobSpec) api.Step {
	return &podStep{
		name:        name,
		config:      config,
		resources:   resources,
		podClient:   podClient,
		artifactDir: artifactDir,
		jobSpec:     jobSpec,
	}
}
