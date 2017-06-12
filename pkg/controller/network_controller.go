package controller

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	tprv1 "github.com/openstack/stackube/pkg/apis/tpr/v1"
	driver "github.com/openstack/stackube/pkg/driver"

	"github.com/golang/glog"
)

// Watcher is an network of watching on resource create/update/delete events
type NetworkController struct {
	NetworkClient     *rest.RESTClient
	NetworkScheme     *runtime.Scheme
	Driver            *driver.OpenStack
	OpenStackTenantID string
}

// Run starts an Network resource controller
func (c *NetworkController) Run(ctx context.Context) error {
	glog.Infof("Begin watching Network objects\n")

	// Watch Network objects
	_, err := c.watchNetworks(ctx)
	if err != nil {
		return fmt.Errorf("Failed to register watch for Network resource: %v\n", err)
	}

	<-ctx.Done()
	return ctx.Err()
}

func (c *NetworkController) watchNetworks(ctx context.Context) (cache.Controller, error) {
	source := cache.NewListWatchFromClient(
		c.NetworkClient,
		tprv1.NetworkResourcePlural,
		apiv1.NamespaceAll,
		fields.Everything())

	_, controller := cache.NewInformer(
		source,

		// The object type.
		&tprv1.Network{},

		// resyncPeriod
		// Every resyncPeriod, all resources in the cache will retrigger events.
		// Set to 0 to disable the resync.
		0,

		// Your custom resource event handlers.
		cache.ResourceEventHandlerFuncs{
			AddFunc:    c.onAdd,
			UpdateFunc: c.onUpdate,
			DeleteFunc: c.onDelete,
		})

	go controller.Run(ctx.Done())
	return controller, nil
}

func (c *NetworkController) onAdd(obj interface{}) {
	network := obj.(*tprv1.Network)
	glog.Infof("[NETWORK CONTROLLER] OnAdd %s\n", network.ObjectMeta.SelfLink)

	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use networkScheme.Copy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	copyObj, err := c.NetworkScheme.Copy(network)
	if err != nil {
		glog.Errorf("ERROR creating a deep copy of network object: %v\n", err)
		return
	}

	networkCopy := copyObj.(*tprv1.Network)

	// This will:
	// 1. Create Network in Neutron
	// 2. Update Network TRP object status to Active or Failed
	c.addNetworkToNeutron(networkCopy)
}

func (c *NetworkController) onUpdate(oldObj, newObj interface{}) {
	oldNetwork := oldObj.(*tprv1.Network)
	newNetwork := newObj.(*tprv1.Network)
	fmt.Printf("[CONTROLLER] OnUpdate oldObj: %s\n", oldNetwork.ObjectMeta.SelfLink)
	fmt.Printf("[CONTROLLER] OnUpdate newObj: %s\n", newNetwork.ObjectMeta.SelfLink)
}

func (c *NetworkController) onDelete(obj interface{}) {
	network := obj.(*tprv1.Network)
	fmt.Printf("[CONTROLLER] OnDelete %s\n", network.ObjectMeta.SelfLink)
}
