/*
Copyright (c) 2017 OpenStack Foundation.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package rbac

import (
	"k8s.io/api/rbac/v1beta1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// GenerateRoleByNamespace generates default-role which has all the permissions in the namespace.
func GenerateRoleByNamespace(namespace string) *v1beta1.Role {
	policyRule := v1beta1.PolicyRule{
		Verbs:     []string{v1beta1.VerbAll},
		APIGroups: []string{v1beta1.APIGroupAll},
		Resources: []string{v1beta1.ResourceAll},
	}
	role := &v1beta1.Role{
		TypeMeta: metav1.TypeMeta{
			Kind:       "Role",
			APIVersion: "rbac.authorization.k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-role",
			Namespace: namespace,
		},
		Rules: []v1beta1.PolicyRule{policyRule},
	}
	return role
}

// GenerateRoleBinding generates rolebinding which allows user "tenant" has deault-role in the tenant namespace.
func GenerateRoleBinding(namespace, tenant string) *v1beta1.RoleBinding {
	subject := v1beta1.Subject{
		Kind: "User",
		Name: tenant,
	}
	roleRef := v1beta1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     "default-role",
	}
	roleBinding := &v1beta1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      tenant + "-rolebinding",
			Namespace: namespace,
		},
		Subjects: []v1beta1.Subject{subject},
		RoleRef:  roleRef,
	}
	return roleBinding
}

// GenerateServiceAccountRoleBinding generates rolebinding of service account in the namespace.
func GenerateServiceAccountRoleBinding(namespace, tenant string) *v1beta1.RoleBinding {
	subject := v1beta1.Subject{
		Kind:      "ServiceAccount",
		Name:      "default",
		Namespace: namespace,
	}
	roleRef := v1beta1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "Role",
		Name:     "default-role",
	}
	roleBinding := &v1beta1.RoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "RoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      tenant + "-rolebinding-sa",
			Namespace: namespace,
		},
		Subjects: []v1beta1.Subject{subject},
		RoleRef:  roleRef,
	}
	return roleBinding
}

// GenerateClusterRole generates namespace-creater ClusterRole which has the permission of namespaces resource.
func GenerateClusterRole() *v1beta1.ClusterRole {
	policyRule := v1beta1.PolicyRule{
		Verbs:     []string{v1beta1.VerbAll},
		APIGroups: []string{v1beta1.APIGroupAll},
		Resources: []string{"namespaces"},
	}

	clusterRole := &v1beta1.ClusterRole{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRole",
			APIVersion: "rbac.authorization.k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: "namespace-creater",
		},
		Rules: []v1beta1.PolicyRule{policyRule},
	}
	return clusterRole
}

// GenerateClusterRoleBindingByTenant generate ClusterRoleBinding which allows anyone in the "tenant" group to create namespace.
func GenerateClusterRoleBindingByTenant(tenant string) *v1beta1.ClusterRoleBinding {
	subject := v1beta1.Subject{
		Kind: "Group",
		Name: tenant,
	}
	roleRef := v1beta1.RoleRef{
		APIGroup: "rbac.authorization.k8s.io",
		Kind:     "ClusterRole",
		Name:     "namespace-creater",
	}

	clusterRoleBinding := &v1beta1.ClusterRoleBinding{
		TypeMeta: metav1.TypeMeta{
			Kind:       "ClusterRoleBinding",
			APIVersion: "rbac.authorization.k8s.io/v1beta1",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name: tenant + "-namespace-creater",
		},
		Subjects: []v1beta1.Subject{subject},
		RoleRef:  roleRef,
	}
	return clusterRoleBinding
}
