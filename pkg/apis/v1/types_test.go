package v1

import (
	"math/rand"
	"testing"

	"github.com/google/gofuzz"

	apitesting "k8s.io/apimachinery/pkg/api/testing"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"
)

var _ runtime.Object = &Network{}
var _ metav1.ObjectMetaAccessor = &Network{}

var _ runtime.Object = &NetworkList{}
var _ metav1.ListMetaAccessor = &NetworkList{}

func networkFuzzerFuncs(t apitesting.TestingCommon) []interface{} {
	return []interface{}{
		func(obj *NetworkList, c fuzz.Continue) {
			c.FuzzNoCustom(obj)
			obj.Items = make([]Network, c.Intn(10))
			for i := range obj.Items {
				c.Fuzz(&obj.Items[i])
			}
		},
	}
}

// TestRoundTrip tests that the third-party kinds can be marshaled and unmarshaled correctly to/from JSON
// without the loss of information. Moreover, deep copy is tested.
func TestRoundTrip(t *testing.T) {
	scheme := runtime.NewScheme()
	codecs := serializer.NewCodecFactory(scheme)

	AddToScheme(scheme)

	seed := rand.Int63()
	fuzzerFuncs := apitesting.MergeFuzzerFuncs(t, apitesting.GenericFuzzerFuncs(t, codecs), networkFuzzerFuncs(t))
	fuzzer := apitesting.FuzzerFor(fuzzerFuncs, rand.NewSource(seed))

	apitesting.RoundTripSpecificKindWithoutProtobuf(t, SchemeGroupVersion.WithKind("Network"), scheme, codecs, fuzzer, nil)
	apitesting.RoundTripSpecificKindWithoutProtobuf(t, SchemeGroupVersion.WithKind("NetworkList"), scheme, codecs, fuzzer, nil)
}
