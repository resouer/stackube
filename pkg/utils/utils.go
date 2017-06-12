package utils

import (
	"errors"
)

const (
	namePrefix = "kube"
)

var ErrNotFound = errors.New("NotFound")
var ErrMultipleResults = errors.New("MultipleResults")

func BuildNetworkName(name, tenantID string) string {
	return namePrefix + "_" + name + "_" + tenantID
}

func BuildLoadBalancerName(name, namespace string) string {
	return namePrefix + "_" + name + "_" + namespace
}
