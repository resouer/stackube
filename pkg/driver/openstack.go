package driver

import (
	"errors"
	"io"
	"os"
	"time"

	"github.com/golang/glog"
	"github.com/gophercloud/gophercloud"
	"github.com/gophercloud/gophercloud/openstack"
	"github.com/gophercloud/gophercloud/openstack/identity/v2/tenants"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/extensions/layer3/routers"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/networks"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/ports"
	"github.com/gophercloud/gophercloud/openstack/networking/v2/subnets"
	"github.com/gophercloud/gophercloud/pagination"
	"github.com/scalingdata/gcfg"

	drivertypes "github.com/openstack/stackube/pkg/driver/types"
)

const (
	podNamePrefix     = "kube"
	securitygroupName = "kube-securitygroup-default"
	hostnameMaxLen    = 63

	// Service affinities
	ServiceAffinityNone     = "None"
	ServiceAffinityClientIP = "ClientIP"
)

var (
	adminStateUp = true

	ErrNotFound        = errors.New("NotFound")
	ErrMultipleResults = errors.New("MultipleResults")
)

// encoding.TextUnmarshaler interface for time.Duration
type MyDuration struct {
	time.Duration
}

func (d *MyDuration) UnmarshalText(text []byte) error {
	res, err := time.ParseDuration(string(text))
	if err != nil {
		return err
	}
	d.Duration = res
	return nil
}

type LoadBalancerOpts struct {
	LBMethod          string     `gcfg:"lb-method"`
	CreateMonitor     bool       `gcfg:"create-monitor"`
	MonitorDelay      MyDuration `gcfg:"monitor-delay"`
	MonitorTimeout    MyDuration `gcfg:"monitor-timeout"`
	MonitorMaxRetries uint       `gcfg:"monitor-max-retries"`
}

//  TODO(harry) Plugin is not used in TPR, but we should consider use a unified openstack library for TPR and CNI
type PluginOpts struct {
	PluginName        string `gcfg:"plugin-name"`
	IntegrationBridge string `gcfg:"integration-bridge"`
}

// OpenStack is an implementation of network provider Interface for OpenStack.
type OpenStack struct {
	network  *gophercloud.ServiceClient
	identity *gophercloud.ServiceClient
	provider *gophercloud.ProviderClient
	region   string
	lbOpts   LoadBalancerOpts
	ExtNetID string
}

type Config struct {
	Global struct {
		AuthUrl    string `gcfg:"auth-url"`
		Username   string `gcfg:"username"`
		UserId     string `gcfg:"user-id"`
		Password   string `gcfg: "password"`
		TokenID    string `gcfg:"token-id"`
		TenantId   string `gcfg:"tenant-id"`
		TenantName string `gcfg:"tenant-name"`
		DomainId   string `gcfg:"domain-id"`
		DomainName string `gcfg:"domain-name"`
		Region     string `gcfg:"region"`
		ExtNetID   string `gcfg:"ext-net-id"`
	}
	LoadBalancer LoadBalancerOpts
}

func (cfg Config) toAuthOptions() gophercloud.AuthOptions {
	return gophercloud.AuthOptions{
		IdentityEndpoint: cfg.Global.AuthUrl,
		Username:         cfg.Global.Username,
		UserID:           cfg.Global.UserId,
		Password:         cfg.Global.Password,
		TokenID:          cfg.Global.TokenID,
		TenantID:         cfg.Global.TenantId,
		TenantName:       cfg.Global.TenantName,

		// Persistent service, so we need to be able to renew tokens.
		AllowReauth: true,
	}
}

func NewOpenStack(config io.Reader) (*OpenStack, error) {
	var cfg Config
	err := gcfg.ReadInto(&cfg, config)
	if err != nil {
		glog.Warning("Failed to parse openstack configure file: %v", err)
		return nil, err
	}

	provider, err := openstack.AuthenticatedClient(cfg.toAuthOptions())
	if err != nil {
		glog.Warning("Failed to auth openstack: %v", err)
		return nil, err
	}

	identity, err := openstack.NewIdentityV2(provider, gophercloud.EndpointOpts{
		Availability: gophercloud.AvailabilityAdmin,
	})
	if err != nil {
		glog.Warning("Failed to find identity endpoint")
		return nil, err
	}

	network, err := openstack.NewNetworkV2(provider, gophercloud.EndpointOpts{
		Region: cfg.Global.Region,
	})
	if err != nil {
		glog.Warning("Failed to find neutron endpoint: %v", err)
		return nil, err
	}

	os := OpenStack{
		identity: identity,
		network:  network,
		provider: provider,
		region:   cfg.Global.Region,
		lbOpts:   cfg.LoadBalancer,
		ExtNetID: cfg.Global.ExtNetID,
	}

	return &os, nil
}

func getHostName() string {
	host, err := os.Hostname()
	if err != nil {
		return ""
	}

	return host
}

// Get openstack network by id
func (os *OpenStack) getOpenStackNetworkByID(id string) (*networks.Network, error) {
	opts := networks.ListOpts{ID: id}
	return os.getOpenStackNetwork(&opts)
}

// Get openstack network by name
func (os *OpenStack) getOpenStackNetworkByName(name string) (*networks.Network, error) {
	opts := networks.ListOpts{Name: name}
	return os.getOpenStackNetwork(&opts)
}

// Get openstack network
func (os *OpenStack) getOpenStackNetwork(opts *networks.ListOpts) (*networks.Network, error) {
	var osNetwork *networks.Network
	pager := networks.List(os.network, *opts)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		networkList, e := networks.ExtractNetworks(page)
		if len(networkList) > 1 {
			return false, ErrMultipleResults
		}

		if len(networkList) == 1 {
			osNetwork = &networkList[0]
		}

		return true, e
	})

	if err == nil && osNetwork == nil {
		return nil, ErrNotFound
	}

	return osNetwork, err
}

// Get provider subnet by id
func (os *OpenStack) getProviderSubnet(osSubnetID string) (*drivertypes.Subnet, error) {
	s, err := subnets.Get(os.network, osSubnetID).Extract()
	if err != nil {
		glog.Errorf("Get openstack subnet failed: %v", err)
		return nil, err
	}

	var routes []*drivertypes.Route
	for _, r := range s.HostRoutes {
		route := drivertypes.Route{
			Nexthop:         r.NextHop,
			DestinationCIDR: r.DestinationCIDR,
		}
		routes = append(routes, &route)
	}

	providerSubnet := drivertypes.Subnet{
		Uid:        s.ID,
		Cidr:       s.CIDR,
		Gateway:    s.GatewayIP,
		Name:       s.Name,
		Dnsservers: s.DNSNameservers,
		Routes:     routes,
	}

	return &providerSubnet, nil
}

// Get network by networkID
func (os *OpenStack) GetNetworkByID(networkID string) (*drivertypes.Network, error) {
	osNetwork, err := os.getOpenStackNetworkByID(networkID)
	if err != nil {
		glog.Errorf("Get openstack network failed: %v", err)
		return nil, err
	}

	return os.OSNetworktoProviderNetwork(osNetwork)
}

// Get network by networkName
func (os *OpenStack) GetNetwork(networkName string) (*drivertypes.Network, error) {
	osNetwork, err := os.getOpenStackNetworkByName(networkName)
	if err != nil {
		glog.Errorf("Get openstack network failed: %v", err)
		return nil, err
	}

	return os.OSNetworktoProviderNetwork(osNetwork)
}

func (os *OpenStack) OSNetworktoProviderNetwork(osNetwork *networks.Network) (*drivertypes.Network, error) {
	var providerNetwork drivertypes.Network
	var providerSubnets []*drivertypes.Subnet
	providerNetwork.Name = osNetwork.Name
	providerNetwork.Uid = osNetwork.ID
	providerNetwork.Status = os.ToProviderStatus(osNetwork.Status)
	providerNetwork.TenantID = osNetwork.TenantID

	for _, subnetID := range osNetwork.Subnets {
		s, err := os.getProviderSubnet(subnetID)
		if err != nil {
			return nil, err
		}
		providerSubnets = append(providerSubnets, s)
	}

	providerNetwork.Subnets = providerSubnets

	return &providerNetwork, nil
}

func (os *OpenStack) ToProviderStatus(status string) string {
	switch status {
	case "ACTIVE":
		return "Active"
	case "BUILD":
		return "Pending"
	case "DOWN", "ERROR":
		return "Failed"
	default:
		return "Failed"
	}

	return "Failed"
}

// Create network
func (os *OpenStack) CreateNetwork(network *drivertypes.Network) error {
	if len(network.Subnets) == 0 {
		return errors.New("Subnets is null")
	}

	// create network
	opts := networks.CreateOpts{
		Name:         network.Name,
		AdminStateUp: &adminStateUp,
		TenantID:     network.TenantID,
	}
	osNet, err := networks.Create(os.network, opts).Extract()
	if err != nil {
		glog.Errorf("Create openstack network %s failed: %v", network.Name, err)
		return err
	}

	// create router
	routerOpts := routers.CreateOpts{
		Name:        network.Name,
		TenantID:    network.TenantID,
		GatewayInfo: &routers.GatewayInfo{NetworkID: os.ExtNetID},
	}
	osRouter, err := routers.Create(os.network, routerOpts).Extract()
	if err != nil {
		glog.Errorf("Create openstack router %s failed: %v", network.Name, err)
		delErr := os.DeleteNetwork(network.Name)
		if delErr != nil {
			glog.Errorf("Delete openstack network %s failed: %v", network.Name, delErr)
		}
		return err
	}

	// create subnets and connect them to router
	networkID := osNet.ID
	network.Status = os.ToProviderStatus(osNet.Status)
	network.Uid = osNet.ID
	for _, sub := range network.Subnets {
		// create subnet
		subnetOpts := subnets.CreateOpts{
			NetworkID:      networkID,
			CIDR:           sub.Cidr,
			Name:           sub.Name,
			IPVersion:      gophercloud.IPv4,
			TenantID:       network.TenantID,
			GatewayIP:      &sub.Gateway,
			DNSNameservers: sub.Dnsservers,
		}
		s, err := subnets.Create(os.network, subnetOpts).Extract()
		if err != nil {
			glog.Errorf("Create openstack subnet %s failed: %v", sub.Name, err)
			delErr := os.DeleteNetwork(network.Name)
			if delErr != nil {
				glog.Errorf("Delete openstack network %s failed: %v", network.Name, delErr)
			}
			return err
		}

		// add subnet to router
		opts := routers.AddInterfaceOpts{
			SubnetID: s.ID,
		}
		_, err = routers.AddInterface(os.network, osRouter.ID, opts).Extract()
		if err != nil {
			glog.Errorf("Create openstack subnet %s failed: %v", sub.Name, err)
			delErr := os.DeleteNetwork(network.Name)
			if delErr != nil {
				glog.Errorf("Delete openstack network %s failed: %v", network.Name, delErr)
			}
			return err
		}
	}

	return nil
}

// Update network
func (os *OpenStack) UpdateNetwork(network *drivertypes.Network) error {
	// TODO: update network subnets
	return nil
}

func (os *OpenStack) getRouterByName(name string) (*routers.Router, error) {
	var result *routers.Router

	opts := routers.ListOpts{Name: name}
	pager := routers.List(os.network, opts)
	err := pager.EachPage(func(page pagination.Page) (bool, error) {
		routerList, e := routers.ExtractRouters(page)
		if len(routerList) > 1 {
			return false, ErrMultipleResults
		} else if len(routerList) == 1 {
			result = &routerList[0]
		}

		return true, e
	})
	if err != nil {
		return nil, err
	}

	return result, nil
}

// Delete network by networkName
func (os *OpenStack) DeleteNetwork(networkName string) error {
	osNetwork, err := os.getOpenStackNetworkByName(networkName)
	if err != nil {
		glog.Errorf("Get openstack network failed: %v", err)
		return err
	}

	if osNetwork != nil {
		// Delete ports
		opts := ports.ListOpts{NetworkID: osNetwork.ID}
		pager := ports.List(os.network, opts)
		err := pager.EachPage(func(page pagination.Page) (bool, error) {
			portList, err := ports.ExtractPorts(page)
			if err != nil {
				glog.Errorf("Get openstack ports error: %v", err)
				return false, err
			}

			for _, port := range portList {
				if port.DeviceOwner == "network:router_interface" {
					continue
				}

				err = ports.Delete(os.network, port.ID).ExtractErr()
				if err != nil {
					glog.Warningf("Delete port %v failed: %v", port.ID, err)
				}
			}

			return true, nil
		})
		if err != nil {
			glog.Errorf("Delete ports error: %v", err)
		}

		router, err := os.getRouterByName(networkName)
		if err != nil {
			glog.Errorf("Get openstack router %s error: %v", networkName, err)
			return err
		}

		// delete all subnets
		for _, subnet := range osNetwork.Subnets {
			if router != nil {
				opts := routers.RemoveInterfaceOpts{SubnetID: subnet}
				_, err := routers.RemoveInterface(os.network, router.ID, opts).Extract()
				if err != nil {
					glog.Errorf("Get openstack router %s error: %v", networkName, err)
					return err
				}
			}

			err = subnets.Delete(os.network, subnet).ExtractErr()
			if err != nil {
				glog.Errorf("Delete openstack subnet %s error: %v", subnet, err)
				return err
			}
		}

		// delete router
		if router != nil {
			err = routers.Delete(os.network, router.ID).ExtractErr()
			if err != nil {
				glog.Errorf("Delete openstack router %s error: %v", router.ID, err)
				return err
			}
		}

		// delete network
		err = networks.Delete(os.network, osNetwork.ID).ExtractErr()
		if err != nil {
			glog.Errorf("Delete openstack network %s error: %v", osNetwork.ID, err)
			return err
		}
	}

	return nil
}

func isNotFound(err error) bool {
	_, ok := err.(*gophercloud.ErrDefault404)
	return ok
}

// Convert tenantID to tenantName
func (os *OpenStack) ToTenantName(tenant string) string {
	opts := tenants.ListOpts{}
	pager := tenants.List(os.identity, &opts)
	result := tenant

	pager.EachPage(func(page pagination.Page) (bool, error) {
		tenantList, err := tenants.ExtractTenants(page)
		if err != nil {
			return false, err
		}

		for _, t := range tenantList {
			if t.ID == tenant {
				result = t.Name
			}
		}

		return true, nil
	})

	return result
}

// Convert tenantName to tenantID
func (os *OpenStack) ToTenantID(tenant string) string {
	opts := tenants.ListOpts{}
	pager := tenants.List(os.identity, &opts)
	result := tenant

	pager.EachPage(func(page pagination.Page) (bool, error) {
		tenantList, err := tenants.ExtractTenants(page)
		if err != nil {
			return false, err
		}

		for _, t := range tenantList {
			if t.Name == tenant {
				result = t.ID
			}
		}

		return true, nil
	})

	return result
}

// Check the tenant id exist
func (os *OpenStack) CheckTenantID(tenantID string) (bool, error) {
	opts := tenants.ListOpts{}
	pager := tenants.List(os.identity, &opts)

	glog.Errorf("harry debug tenantID: %v", tenantID)

	var found bool
	err := pager.EachPage(func(page pagination.Page) (bool, error) {

		tenantList, err := tenants.ExtractTenants(page)
		if err != nil {
			return false, err
		}

		glog.Errorf("harry debug tenantList: %v", tenantList)

		if len(tenantList) == 0 {
			return false, ErrNotFound
		}

		for _, t := range tenantList {
			if t.ID == tenantID || t.Name == tenantID {
				found = true
			}
		}

		return true, nil
	})

	return found, err
}

func (os *OpenStack) BuildPortName(podName, namespace, networkID string) string {
	return podNamePrefix + "_" + podName + "_" + namespace + "_" + networkID
}
