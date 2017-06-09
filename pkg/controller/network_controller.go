package controller

import (
	"context"

	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"

	"github.com/golang/glog"

	tprv1 "github.com/openstack/stackube/pkg/apis/v1"
)

// Watcher is an network of watching on resource create/update/delete events
type NetworkController struct {
	NetworkClient *rest.RESTClient
	NetworkScheme *runtime.Scheme
}

// Run starts an Network resource controller
func (c *NetworkController) Run(ctx context.Context) error {
	glog.Infof("Watch Network objects\n")

	// Watch Network objects
	_, err := c.watchNetworks(ctx)
	if err != nil {
		glog.Errorf("Failed to register watch for Network resource: %v\n", err)
		return err
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
	glog.Infof("[CONTROLLER] OnAdd %s\n", network.ObjectMeta.SelfLink)

	// NEVER modify objects from the store. It's a read-only, local cache.
	// You can use networkScheme.Copy() to make a deep copy of original object and modify this copy
	// Or create a copy manually for better performance
	copyObj, err := c.NetworkScheme.Copy(network)
	if err != nil {
		glog.Errorf("ERROR creating a deep copy of network object: %v\n", err)
		return
	}

	networkCopy := copyObj.(*tprv1.Network)
	networkCopy.Status = tprv1.NetworkStatus{
		State:   tprv1.NetworkStateProcessed,
		Message: "Successfully processed by controller",
	}

	// TODO(harry) What to do next? Who add network object?
	err = c.NetworkClient.Put().
		Name(network.ObjectMeta.Name).
		Namespace(network.ObjectMeta.Namespace).
		Resource(tprv1.NetworkResourcePlural).
		Body(networkCopy).
		Do().
		Error()

	if err != nil {
		glog.Errorf("ERROR updating status: %v\n", err)
	} else {
		glog.Infof("UPDATED status: %#v\n", networkCopy)
	}
}

func (c *NetworkController) onUpdate(oldObj, newObj interface{}) {
	oldNetwork := oldObj.(*tprv1.Network)
	newNetwork := newObj.(*tprv1.Network)
	glog.Infof("[CONTROLLER] OnUpdate oldObj: %s\n", oldNetwork.ObjectMeta.SelfLink)
	glog.Infof("[CONTROLLER] OnUpdate newObj: %s\n", newNetwork.ObjectMeta.SelfLink)
}

func (c *NetworkController) onDelete(obj interface{}) {
	network := obj.(*tprv1.Network)
	glog.Infof("[CONTROLLER] OnDelete %s\n", network.ObjectMeta.SelfLink)
}
