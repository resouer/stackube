package controller

import (
	"k8s.io/apimachinery/pkg/util/uuid"

	tprv1 "github.com/openstack/stackube/pkg/apis/tpr/v1"
	drivertypes "github.com/openstack/stackube/pkg/driver/types"
	"github.com/openstack/stackube/pkg/utils"

	"github.com/golang/glog"
)

const (
	subnetPrefix  = "subnet"
	networkPrefix = "network"
)

func (c *NetworkController) addNetworkToNeutron(kubeNetwork *tprv1.Network) {
	// Translate Kubernetes network to OpenStack network
	driverNetwork := &drivertypes.Network{
		Name:     kubeNetwork.GetName() + "_" + networkPrefix,
		Uid:      string(kubeNetwork.GetUID()),
		TenantID: c.OpenStackTenantID,
		Subnets: []*drivertypes.Subnet{
			{
				// network: subnet = 1:1
				Name:    kubeNetwork.GetName() + "_" + subnetPrefix,
				Uid:     string(uuid.NewUUID()),
				Cidr:    kubeNetwork.Spec.CIDR,
				Gateway: kubeNetwork.Spec.Gateway,
			},
		},
		Status: tprv1.NetworkInitializing,
	}

	newNetworkStatus := tprv1.NetworkActive

	glog.V(4).Infof("[NetworkOperator]: add network %s", driverNetwork.Name)

	// Check if tenant id exist
	check, err := c.Driver.CheckTenantID(driverNetwork.TenantID)
	if err != nil {
		glog.Errorf("[NetworkOperator]: check tenantID failed: %v", err)
	}
	if !check {
		glog.Warningf("[NetworkOperator]: tenantID %s doesn't exit in network provider", driverNetwork.TenantID)
		kubeNetwork.Status.State = tprv1.NetworkFailed
		c.updateNetwork(kubeNetwork)
		return
	}

	// Check if provider network id exist
	if kubeNetwork.Spec.NetworkID != "" {
		_, err := c.Driver.GetNetworkByID(kubeNetwork.Spec.NetworkID)
		if err != nil {
			glog.Warningf("[NetworkOperator]: network %s doesn't exit in network provider", kubeNetwork.Spec.NetworkID)
			newNetworkStatus = tprv1.NetworkFailed
		}
	} else {
		if len(driverNetwork.Subnets) == 0 {
			glog.Warningf("[NetworkOperator]: subnets of %s is null", driverNetwork.Name)
			newNetworkStatus = tprv1.NetworkFailed
		} else {
			// Check if provider network has already created
			networkName := utils.BuildNetworkName(driverNetwork.Name, driverNetwork.TenantID)
			_, err := c.Driver.GetNetwork(networkName)
			if err == nil {
				glog.Infof("[NetworkOperator]: network %s has already created", networkName)
			} else if err.Error() == utils.ErrNotFound.Error() {
				// Create a new network by network provider
				err := c.Driver.CreateNetwork(driverNetwork)
				if err != nil {
					glog.Warningf("[NetworkOperator]: create network %s failed: %v", driverNetwork.Name, err)
					newNetworkStatus = tprv1.NetworkFailed
				}
			} else {
				glog.Warningf("[NetworkOperator]: get network failed: %v", err)
				newNetworkStatus = tprv1.NetworkFailed
			}
		}
	}

	kubeNetwork.Status.State = newNetworkStatus
	c.updateNetwork(kubeNetwork)
}

// updateNetwork updates Network TPR object by given object
func (c *NetworkController) updateNetwork(network *tprv1.Network) {
	err := c.NetworkClient.Put().
		Name(network.ObjectMeta.Name).
		Namespace(network.ObjectMeta.Namespace).
		Resource(tprv1.NetworkResourcePlural).
		Body(network).
		Do().
		Error()

	if err != nil {
		glog.Errorf("ERROR updating network status: %v\n", err)
	} else {
		glog.Infof("UPDATED network status: %#v\n", network)
	}
}
