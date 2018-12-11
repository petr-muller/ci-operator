package steps

// This file contains helpers useful for testing ci-operator steps

import (
	"context"
	"reflect"
	"testing"

	apibuildv1 "github.com/openshift/api/build/v1"
	buildv1 "github.com/openshift/client-go/build/clientset/versioned/typed/build/v1"

	apiimagev1 "github.com/openshift/api/image/v1"
	imagev1 "github.com/openshift/client-go/image/clientset/versioned/typed/image/v1"

	fakebuildclientset "github.com/openshift/client-go/build/clientset/versioned/fake"
	fakeimageclientset "github.com/openshift/client-go/image/clientset/versioned/fake"
	fakerouteclientset "github.com/openshift/client-go/route/clientset/versioned/fake"

	apicorev1 "k8s.io/api/core/v1"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	fakecorev1 "k8s.io/client-go/kubernetes/typed/core/v1/fake"

	"k8s.io/apimachinery/pkg/util/diff"
	"k8s.io/client-go/kubernetes/fake"

	"github.com/openshift/ci-operator/pkg/api"
)

func createTestingClient(t *testing.T) *ciopTestingClient {
	return &ciopTestingClient{
		kubecs:  fake.NewSimpleClientset(),
		imagecs: fakeimageclientset.NewSimpleClientset(),
		routecs: fakerouteclientset.NewSimpleClientset(),
		buildcs: fakebuildclientset.NewSimpleClientset(),
		t:       t,
	}
}

// Fake Clientset, created so we can override its `Core()` method
// and return our fake CoreV1 API (=ciopTestingCore)

type ciopTestingClient struct {
	kubecs  *fake.Clientset
	imagecs *fakeimageclientset.Clientset
	routecs *fakerouteclientset.Clientset
	buildcs *fakebuildclientset.Clientset
	t       *testing.T
}

func (c *ciopTestingClient) Core() corev1.CoreV1Interface {
	fc := c.kubecs.Core().(*fakecorev1.FakeCoreV1)
	return &ciopTestingCore{*fc, c.t}
}

// Fake CoreV1, created so we can override its `Pods()` method
// and return our fake Pods API (=ciopTestingPods)

type ciopTestingCore struct {
	fakecorev1.FakeCoreV1
	t *testing.T
}

func (c *ciopTestingCore) Pods(ns string) corev1.PodInterface {
	pods := c.FakeCoreV1.Pods(ns).(*fakecorev1.FakePods)
	return &ciopTestingPods{*pods, c.t}
}

// Fake Pods API

type ciopTestingPods struct {
	fakecorev1.FakePods
	t *testing.T
}

// Fake Create() provided by the lib creates objects without default values, so
// they would be created without any sensible Phase, which causes problems in
// the ci-operator code. Therefore, our fake Create() always creates Pods with
// a `Pending` phase if it does not carry phase already.
func (c *ciopTestingPods) Create(pod *apicorev1.Pod) (*apicorev1.Pod, error) {
	c.t.Logf("FakePods.Create(): ObjectMeta.Name=%s Status.Phase=%s", pod.ObjectMeta.Name, pod.Status.Phase)
	if pod.Status.Phase == "" {
		pod.Status.Phase = apicorev1.PodPending
		c.t.Logf("FakePods.Create(): Setting Status.Phase to '%s'", apicorev1.PodPending)
	}
	return c.FakePods.Create(pod)
}

type doneExpectation struct {
	value bool
	err   bool
}

type providesExpectation struct {
	params map[string]string
	link   api.StepLink
}

type inputsExpectation struct {
	values api.InputDefinition
	err    bool
}

type stepExpectation struct {
	name     string
	requires []api.StepLink
	creates  []api.StepLink
	provides providesExpectation
	inputs   inputsExpectation
}

type executionExpectation struct {
	prerun   doneExpectation
	runError bool
	postrun  doneExpectation
}

func someStepLink(as string) api.StepLink {
	return api.ExternalImageLink(api.ImageStreamTagReference{
		Cluster:   "cluster.com",
		Namespace: "namespace",
		Name:      "name",
		Tag:       "tag",
		As:        as,
	})
}

func fakeSuccessfulBuild(build *apibuildv1.Build, buildclient buildv1.BuildV1Interface, istclient imagev1.ImageV1Interface, t *testing.T) {
	if build.Spec.Output.To.Kind == "ImageStreamTag" {
		name := build.Spec.Output.To.Name
		namespace := build.Spec.Output.To.Namespace
		t.Logf("Fake cluster: Build output is ImageStreamTag '%s' in namespace '%s': creating a fake ImageStreamTag with a fake Image", name, namespace)
		istag := &apiimagev1.ImageStreamTag{
			ObjectMeta: meta.ObjectMeta{
				Name:      name,
				Namespace: namespace,
			},
			Image: apiimagev1.Image{ObjectMeta: meta.ObjectMeta{Name: "DoesThisMatter"}},
		}
		if _, err := istclient.ImageStreamTags(namespace).Create(istag); err != nil {
			t.Errorf("Fake cluster: Failed to treate ImageStreamTag to fake Build output: %v", err)
			return
		}
	}

	// make a copy to avoid a race
	newBuild := build.DeepCopy()
	newBuild.Status.Phase = apibuildv1.BuildPhaseComplete
	if _, err := buildclient.Builds("jobspecNamespace").UpdateStatus(newBuild); err != nil {
		t.Errorf("Fake cluster: UpdateStatus() returned an error: %v", err)
	}
}

func errorCheck(t *testing.T, message string, expected bool, err error) {
	if expected && err == nil {
		t.Errorf("%s: expected to return error, returned nil", message)
	}
	if !expected && err != nil {
		t.Errorf("%s: returned unexpected error: %v", message, err)
	}
}

func examineStep(t *testing.T, step api.Step, expected stepExpectation) {
	// Test the "informative" methods
	if name := step.Name(); name != expected.name {
		t.Errorf("step.Name() mismatch: expected '%s', got '%s'", expected.name, name)
	}
	if desc := step.Description(); len(desc) == 0 {
		t.Errorf("step.Description() returned an empty string")
	}
	if reqs := step.Requires(); !reflect.DeepEqual(expected.requires, reqs) {
		t.Errorf("step.Requires() returned different links:\n%s", diff.ObjectReflectDiff(expected.requires, reqs))
	}
	if creates := step.Creates(); !reflect.DeepEqual(expected.creates, creates) {
		t.Errorf("step.Creates() returned different links:\n%s", diff.ObjectReflectDiff(expected.creates, creates))
	}

	params, link := step.Provides()
	for expectedKey, expectedValue := range expected.provides.params {
		getFunc, ok := params[expectedKey]
		if !ok {
			t.Errorf("step.Provides: Parameters do not contain '%s' key (expected to return value '%s')", expectedKey, expectedValue)
		}
		value, err := getFunc()
		if err != nil {
			t.Errorf("step.Provides: params[%s]() returned error: %v", expectedKey, err)
		} else if value != expectedValue {
			t.Errorf("step.Provides: params[%s]() returned '%s', expected to return '%s'", expectedKey, value, expectedValue)
		}
	}
	if !reflect.DeepEqual(expected.provides.link, link) {
		t.Errorf("step.Provides returned different link\n%s", diff.ObjectReflectDiff(expected.provides.link, link))
	}

	inputs, err := step.Inputs(context.Background(), false)
	if !reflect.DeepEqual(expected.inputs.values, inputs) {
		t.Errorf("step.Inputs returned different inputs\n%s", diff.ObjectReflectDiff(expected.inputs.values, inputs))
	}
	errorCheck(t, "step.Inputs", expected.inputs.err, err)
}

func executeStep(t *testing.T, step api.Step, expected executionExpectation, fakeClusterBehavior func()) {
	done, err := step.Done()
	if !reflect.DeepEqual(expected.prerun.value, done) {
		t.Errorf("step.Done() before Run() returned %t, expected %t)", done, expected.prerun.value)
	}
	errorCheck(t, "step.Done() before Run()", expected.prerun.err, err)

	if fakeClusterBehavior != nil {
		go fakeClusterBehavior()
	}

	err = step.Run(context.Background(), false)
	errorCheck(t, "step.Run()", expected.runError, err)

	done, err = step.Done()
	if !reflect.DeepEqual(expected.postrun.value, done) {
		t.Errorf("step.Done() after Run() returned %t, expected %t)", done, expected.postrun.value)
	}
	errorCheck(t, "step.Done() after Run()", expected.postrun.err, err)
}
