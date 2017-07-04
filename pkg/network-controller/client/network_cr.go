package client

import (
	"reflect"
	"time"

	crv1 "git.openstack.org/openstack/stackube/pkg/apis/v1"
	"git.openstack.org/openstack/stackube/pkg/util"

	apiextensionsv1beta1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1beta1"
	apiextensionsclient "k8s.io/apiextensions-apiserver/pkg/client/clientset/clientset"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	apiv1 "k8s.io/client-go/pkg/api/v1"
	"k8s.io/client-go/rest"
)

const (
	networkCRDName = crv1.NetworkResourcePlural + "." + crv1.GroupName
)

func CreateNetworkCRD(clientset apiextensionsclient.Interface) (*apiextensionsv1beta1.CustomResourceDefinition, error) {
	crd := &apiextensionsv1beta1.CustomResourceDefinition{
		ObjectMeta: metav1.ObjectMeta{
			Name: networkCRDName,
		},
		Spec: apiextensionsv1beta1.CustomResourceDefinitionSpec{
			Group:   crv1.GroupName,
			Version: crv1.SchemeGroupVersion.Version,
			Scope:   apiextensionsv1beta1.NamespaceScoped,
			Names: apiextensionsv1beta1.CustomResourceDefinitionNames{
				Plural: crv1.NetworkResourcePlural,
				Kind:   reflect.TypeOf(crv1.Network{}).Name(),
			},
		},
	}
	_, err := clientset.ApiextensionsV1beta1().CustomResourceDefinitions().Create(crd)
	if err != nil {
		return nil, err
	}

	// wait for CRD being established
	if err = util.WaitForCRDReady(clientset, networkCRDName); err != nil {
		return nil, err
	} else {
		return crd, nil
	}
}

func WaitForNetworkInstanceProcessed(kubeClient *rest.RESTClient, name string) error {
	return wait.Poll(100*time.Millisecond, 10*time.Second, func() (bool, error) {
		var network crv1.Network
		err := kubeClient.Get().
			Resource(crv1.NetworkResourcePlural).
			Namespace(apiv1.NamespaceDefault).
			Name(name).
			Do().Into(&network)

		if err == nil && network.Status.State == crv1.NetworkActive {
			return true, nil
		}

		return false, err
	})
}
