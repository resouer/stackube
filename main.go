package main

import (
	"context"
	"flag"
	"fmt"

	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	"github.com/golang/glog"
	tprv1 "github.com/openstack/stackube/pkg/apis/v1"
	networkclient "github.com/openstack/stackube/pkg/client"
	networkcontroller "github.com/openstack/stackube/pkg/controller"
)

func main() {
	kubeconfig := flag.String("kubeconfig", "", "Path to a kube config. Only required if out-of-cluster.")
	flag.Parse()

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(*kubeconfig)
	if err != nil {
		glog.Fatalf("failed to build kubeconfig", err)
	}

	apiextensionsclientset, err := apiextensionsclient.NewForConfig(config)
	if err != nil {
		glog.Fatalf("failed to create kubeconfig", err)
	}

	// initialize custom resource using a CustomResourceDefinition if it does not exist
	crd, err := networkclient.CreateCustomResourceDefinition(apiextensionsclientset)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		glog.Fatalf("failed to create CRD", err)
	}
	defer apiextensionsclientset.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(crd.Name, nil)

	// make a new config for our extension's API group, using the first config as a baseline
	networkClient, networkScheme, err := networkclient.NewClient(config)
	if err != nil {
		glog.Fatalf("failed to make a new configure CRD", err)
	}

	// start a controller on instances of our custom resource
	controller := networkcontroller.NetworkController{
		NetworkClient: networkClient,
		NetworkScheme: networkScheme,
	}

	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()
	go controller.Run(ctx)

	// Create an instance of our custom resource
	network := &tprv1.Network{
		ObjectMeta: metav1.ObjectMeta{
			Name: "network1",
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
		glog.Infof("CREATED: %#v\n", result)
	} else if apierrors.IsAlreadyExists(err) {
		glog.Infof("ALREADY EXISTS: %#v\n", result)
	} else {
		glog.Fatalf("create network object failed: %v", err)
	}

	// Poll until Network object is handled by controller and gets status updated to "Processed"
	err = networkclient.WaitForNetworkInstanceProcessed(networkClient, "network1")
	if err != nil {
		glog.Fatalf("failed to wait for Network object processed: %v", err)
	}
	fmt.Printf("PROCESSED\n")

	// Fetch a list of our TPRs
	networkList := tprv1.NetworkList{}
	err = networkClient.Get().Resource(tprv1.NetworkResourcePlural).Do().Into(&networkList)
	if err != nil {
		glog.Fatalf("failed to get Network object list", err)
	}
	glog.Infof("LIST: %#v\n", networkList)
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
