package client

import (
	"reflect"
	"time"

	tprv1 "github.com/openstack/stackube/pkg/apis/v1"
	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/errors"
	"k8s.io/apimachinery/pkg/util/wait"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"

	"github.com/golang/glog"
)

const networkCRDName = tprv1.NetworkResourcePlural + "." + tprv1.GroupName

func CreateCustomResourceDefinition(clientset apiextensionsclient.Interface) (*apiextensionsv1beta1.CustomResourceDefinition, error) {
	network := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: networkCRDName,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   tprv1.GroupName,
			Version: tprv1.SchemeGroupVersion.Version,
			Scope:   apiextensionsv1beta1.NamespaceScoped,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural: tprv1.NetworkResourcePlural,
				Kind:   reflect.TypeOf(tprv1.Network{}).Name(),
			},
		},
	}
	_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(network)
	if err != nil {
		return nil, err
	}

	// wait for CRD being established
	err = wait.Poll(500*time.Millisecond, 60*time.Second, func() (bool, error) {
		network, err = clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Get(networkCRDName, metav1.GetOptions{})
		if err != nil {
			return false, err
		}
		for _, cond := range network.Status.Conditions {
			switch cond.Type {
			case apiextensionsv1beta1.Established:
				if cond.Status == apiextensionsv1beta1.ConditionTrue {
					return true, err
				}
			case apiextensionsv1beta1.NamesAccepted:
				if cond.Status == apiextensionsv1beta1.ConditionFalse {
					glog.Errorf("CRD Name conflict: %v\n", cond.Reason)
				}
			}
		}
		return false, err
	})
	if err != nil {
		deleteErr := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Delete(networkCRDName, nil)
		if deleteErr != nil {
			return nil, errors.NewAggregate([]error{err, deleteErr})
		}
		return nil, err
	}
	return network, nil
}

// TODO(harry) Do we need add create delete update etc?
func WaitForNetworkInstanceProcessed(networkClient *rest.RESTClient, name string) error {
	return wait.Poll(100*time.Millisecond, 10*time.Second, func() (bool, error) {
		var network tprv1.Network
		err := networkClient.Get().
			Resource(tprv1.NetworkResourcePlural).
			Namespace(apiv1.NamespaceDefault).
			Name(name).
			Do().Into(&network)

		if err == nil && network.Status.State == tprv1.NetworkStateProcessed {
			return true, nil
		}

		return false, err
	})
}
