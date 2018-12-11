package steps

import (
	"testing"

	buildv1 "github.com/openshift/api/build/v1"
	routev1 "github.com/openshift/api/route/v1"

	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	fakerest "k8s.io/client-go/rest/fake"

	"github.com/openshift/ci-operator/pkg/api"
)

func TestRPMImageInjectionStep(t *testing.T) {
	config := api.RPMImageInjectionStepConfiguration{
		From: "configFrom",
		To:   "configTo",
	}
	resources := api.ResourceConfiguration{}

	fakecs := createTestingClient(t)

	istclient := fakecs.imagecs.ImageV1()
	routeclient := fakecs.routecs.RouteV1()
	buildsgetter := fakecs.buildcs.BuildV1()
	restclient := &fakerest.RESTClient{}
	buildclient := NewBuildClient(buildsgetter, restclient)

	jobspec := &api.JobSpec{Namespace: "jobspecNamespace"}

	rpmiis := RPMImageInjectionStep(config, resources, buildclient, routeclient, istclient, jobspec)

	specification := stepExpectation{
		name: "configTo",
		requires: []api.StepLink{
			api.InternalImageLink("configFrom"),
			api.RPMRepoLink(),
		},
		creates:  []api.StepLink{api.InternalImageLink("configTo")},
		provides: providesExpectation{params: nil, link: nil},
		inputs:   inputsExpectation{values: nil, err: false},
	}

	examineStep(t, rpmiis, specification)

	rpmroute := &routev1.Route{
		ObjectMeta: meta.ObjectMeta{Name: RPMRepoName},
	}
	routeclient.Routes(jobspec.Namespace).Create(rpmroute)

	watcher, err := buildclient.Builds("jobspecNamespace").Watch(meta.ListOptions{})
	if err != nil {
		t.Errorf("Failed to create a watcher over Builds in namespace 'jobspecNamespace'")
	}
	defer watcher.Stop()

	clusterBehavior := func() {
		// Expect a single event (a Creation) to happen
		// Immediately set the Build status to Complete, because
		// that is what the step waits on
		for {
			event, ok := <-watcher.ResultChan()
			if !ok {
				t.Error("Fake cluster: watcher event closed, exiting")
				break
			}
			if build, ok := event.Object.(*buildv1.Build); ok {
				t.Logf("Fake cluster: Received event on Build '%s': %s", build.ObjectMeta.Name, event.Type)
				t.Logf("Fake cluster: Updating build '%s' status to '%s' and exiting", build.ObjectMeta.Name, buildv1.BuildPhaseComplete)
				fakeSuccessfulBuild(build, buildsgetter, istclient, t)
				break
			}
			t.Logf("Fake cluster: Received non-build event: %v", event)
		}
	}

	execSpec := executionExpectation{
		prerun:   doneExpectation{value: false, err: false},
		runError: false,
		postrun:  doneExpectation{value: true, err: false},
	}

	executeStep(t, rpmiis, execSpec, clusterBehavior)
	// No further checks needed because we would only check for resources which
	// the fake cluster behavior creates
}
