package scale

import (
	"context"
	"strconv"
	"time"

	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	v12 "kubevirt.io/api/core/v1"

	"kubevirt.io/kubevirt/tests/decorators"
	"kubevirt.io/kubevirt/tests/flags"
	"kubevirt.io/kubevirt/tests/framework/kubevirt"
	"kubevirt.io/kubevirt/tests/util"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"kubevirt.io/client-go/kubecli"

	"kubevirt.io/kubevirt/pkg/apimachinery/patch"
	"kubevirt.io/kubevirt/pkg/virt-operator/resource/generate/components"
)

var _ = Describe("[sig-compute] virt-api scaling", decorators.SigCompute, func() {
	var virtClient kubecli.KubevirtClient
	numberOfNodes := 0

	setccs := func(ccs v12.CustomizeComponents, kvNamespace string, kvName string) error {
		patchPayload, err := patch.New(patch.WithReplace("/spec/customizeComponents", ccs)).GeneratePayload()
		if err != nil {
			return err
		}
		_, err = virtClient.KubeVirt(kvNamespace).Patch(context.Background(), kvName, types.JSONPatchType, patchPayload, v1.PatchOptions{})
		return err
	}

	getApiReplicas := func(virtClient kubecli.KubevirtClient, expectedResult int32) int32 {
		By("Finding out virt-api replica number")
		apiDeployment, err := virtClient.AppsV1().Deployments(flags.KubeVirtInstallNamespace).Get(context.Background(), components.VirtAPIName, v1.GetOptions{})
		Expect(err).ToNot(HaveOccurred())

		By("Expecting number of replicas to be as expected")
		Expect(apiDeployment.Spec.Replicas).ToNot(BeNil())

		return *apiDeployment.Spec.Replicas
	}
	BeforeEach(func() {
		virtClient = kubevirt.Client()
		if numberOfNodes == 0 {
			By("Finding out nodes count")
			nodes, err := virtClient.CoreV1().Nodes().List(context.Background(), v1.ListOptions{})
			Expect(err).ToNot(HaveOccurred())

			numberOfNodes = len(nodes.Items)
		}
	})

	calcExpectedReplicas := func(nodesCount int) (expectedReplicas int32) {
		// Please note that this logic is temporary. For more information take a look on the comment in
		// getDesiredApiReplicas() function in pkg/virt-operator/resource/apply/apps.go.
		//
		// When the logic is replaced for getDesiredApiReplicas(), it needs to be replaced here as well.

		if nodesCount == 1 {
			return 1
		}

		const minReplicas = 2

		expectedReplicas = int32(nodesCount) / 10
		if expectedReplicas < minReplicas {
			expectedReplicas = minReplicas
		}

		return expectedReplicas
	}

	It("virt-api replicas should be scaled as expected", func() {
		By("Finding out nodes count")
		expectedResult := calcExpectedReplicas(numberOfNodes)
		Eventually(func() int32 {
			return getApiReplicas(virtClient, expectedResult)
		}, 1*time.Minute, 5*time.Second).Should(Equal(calcExpectedReplicas(numberOfNodes)), "number of virt API should be as expected")
	})

	It("[Serial]virt-api replicas should be determined by patch if exist", Serial, func() {
		originalKv := util.GetCurrentKv(virtClient)
		expectedResult := calcExpectedReplicas(numberOfNodes)
		expectedResult += 1
		ccs := v12.CustomizeComponents{
			Patches: []v12.CustomizeComponentsPatch{
				{
					ResourceName: components.VirtAPIName,
					ResourceType: "Deployment",
					Patch:        `[{"op":"replace","path":"/spec/replicas","value":` + strconv.Itoa(int(expectedResult)) + `}]`,
					Type:         v12.JSONPatchType,
				},
			},
		}
		err := setccs(ccs, originalKv.Namespace, originalKv.Name)
		Expect(err).ToNot(HaveOccurred())
		DeferCleanup(func() {
			err := setccs(originalKv.Spec.CustomizeComponents, originalKv.Namespace, originalKv.Name)
			Expect(err).ToNot(HaveOccurred())
		})

		Eventually(func() int32 {
			return getApiReplicas(virtClient, expectedResult)
		}, 1*time.Minute, 5*time.Second).Should(Equal(expectedResult), "number of virt API should be as expected")
	})

})
