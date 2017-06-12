package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"

	tprclient "github.com/openstack/stackube/pkg/client"
	tprcontroller "github.com/openstack/stackube/pkg/controller"
	"github.com/openstack/stackube/pkg/driver"
)

var (
	kubeconfig          = flag.String("kubeconfig", "/etc/kubernetes/admin.conf", "Path to a kube config. Only required if out-of-cluster.")
	openstackConfigFile = flag.String("openstackconfig", "/etc/kubestack.conf", "Path to a OpenStack config.")
)

func main() {
	flag.Parse()
	defer utilruntime.HandleCrash()

	// Read OpenStack configuration file
	openstackConfig, err := os.Open(*openstackConfigFile)
	if err != nil {
		fmt.Printf("Couldn't open configuration file %s: %#v", openstackConfigFile, err)
		os.Exit(1)
	}
	defer openstackConfig.Close()

	// Create OpenStack client from config
	openstack, err := driver.NewOpenStack(openstackConfig)
	if err != nil {
		fmt.Printf("Couldn't initialize openstack: %#v", err)
		os.Exit(1)
	}

	// Create the client config. Use kubeconfig if given, otherwise assume in-cluster.
	config, err := buildConfig(*kubeconfig)
	if err != nil {
		fmt.Printf("failed to build kubeconfig: %v", err)
		os.Exit(1)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		fmt.Printf("failed to create kubeclient from config: %v", err)
		os.Exit(1)
	}

	// initialize third party resource if it does not exist
	err = tprclient.CreateNetworkTPR(clientset)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		fmt.Printf("failed to create TPR to kube-apiserver: %v", err)
		os.Exit(1)
	}

	// make a new config for our extension's API group, using the first config as a baseline
	networkClient, networkScheme, err := tprclient.NewClient(config)
	if err != nil {
		fmt.Printf("failed to create client for TPR: %v", err)
		os.Exit(1)
	}

	// wait until TPR gets processed
	err = tprclient.WaitForNetworkResource(networkClient)
	if err != nil {
		fmt.Printf("failed to wait TPR change to ready status: %v", err)
		os.Exit(1)
	}

	// TODO(harry) fix this when we can get tenantID
	openstackTenantID := os.Getenv("TENANTID")
	if len(openstackTenantID) == 0 {
		fmt.Printf("OpenStack should be provided")
		os.Exit(1)
	}

	controller := tprcontroller.NetworkController{
		NetworkClient:     networkClient,
		NetworkScheme:     networkScheme,
		Driver:            openstack,
		OpenStackTenantID: string(openstackTenantID),
	}

	// start a controller on instances of our TPR
	ctx, cancelFunc := context.WithCancel(context.Background())
	defer cancelFunc()

	controller.Run(ctx)
}

func buildConfig(kubeconfig string) (*rest.Config, error) {
	if kubeconfig != "" {
		return clientcmd.BuildConfigFromFlags("", kubeconfig)
	}
	return rest.InClusterConfig()
}
