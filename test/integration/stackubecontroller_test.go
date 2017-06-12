package integration

import (
	"os"
	"testing"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	tprv1 "github.com/openstack/stackube/pkg/apis/tpr/v1"
	tprclient "github.com/openstack/stackube/pkg/client"
)

const (
	KUBE_CONFIG_PATH = "/etc/kubernetes/admin.conf"
)

// This integration test act as a TPR client to CURD network TPR
// A existing Kubernetes configure should be passed in
func TestStackubeController(t *testing.T) {
	kubeconfig := os.Getenv("KUBECONFIG")
	if len(kubeconfig) == 0 {
		// default to kubeadm config path
		kubeconfig = KUBE_CONFIG_PATH
	}
	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(kubeconfig)
	if err != nil {
		t.Fatal(err)
	}

	// make a new config for our extension's API group, using the first config as a baseline
	networkClient, _, err := tprclient.NewClient(config)
	if err != nil {
		t.Fatal(err)
	}

	// wait until TPR gets processed, this should always work
	err = tprclient.WaitForNetworkResource(networkClient)
	if err != nil {
		t.Fatal(err)
	}

	// Create an instance of our TPR
	networkName := "network1"
	network := &tprv1.Network{
		ObjectMeta: metav1.ObjectMeta{
			// in real world, use "namespace + networkName"
			Name: networkName,
		},
		Spec: tprv1.NetworkSpec{
			CIDR:    "10.0.0.1/16",
			Gateway: "10.0.0.1",
		},
		Status: tprv1.NetworkStatus{
			State:   tprv1.NetworkStateCreated,
			Message: "Network object created, not processed yet",
		},
	}
	var result tprv1.Network
	err = networkClient.Post().
		Resource(tprv1.NetworkResourcePlural).
		Namespace(apiv1.NamespaceDefault).
		Body(network).
		Do().Into(&result)
	if err == nil {
		t.Logf("CREATED: %#v\n", result)
	} else if apierrors.IsAlreadyExists(err) {
		t.Logf("ALREADY EXISTS: %#v\n", result)
	} else {
		t.Fatal(err)
	}

	// Poll until Network object is handled by controller and gets status updated to NetworkActive
	err = tprclient.WaitForNetworkInstanceProcessed(networkClient, networkName)
	if err != nil {
		t.Fatal(err)
	}
	t.Log("NetworkActive\n")

	// Fetch a list of our TPRs
	networkList := tprv1.NetworkList{}
	err = networkClient.Get().Resource(tprv1.NetworkResourcePlural).Do().Into(&networkList)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("LIST: %#v\n", networkList)

	err = networkClient.Delete().Resource(tprv1.NetworkResourcePlural).
		Namespace(apiv1.NamespaceDefault).Name(networkName).Do().Into(&result)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("DELETE: %#v\n", networkName)
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
