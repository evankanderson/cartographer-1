// Copyright 2021 VMware
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package workload

import (
	"fmt"

	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"

	"github.com/vmware-tanzu/cartographer/pkg/apis/v1alpha1"
	"github.com/vmware-tanzu/cartographer/pkg/utils"
)

type GetClusterTemplateError struct {
	Err         error
	TemplateRef v1alpha1.ClusterTemplateReference
}

func (e GetClusterTemplateError) Error() string {
	return fmt.Errorf("unable to get template [%s]: %w", e.TemplateRef.Name, e.Err).Error()
}

type ApplyStampedObjectError struct {
	Err           error
	StampedObject *unstructured.Unstructured
}

func (e ApplyStampedObjectError) Error() string {
	return fmt.Errorf("unable to apply object [%s/%s]: %w", e.StampedObject.GetNamespace(), e.StampedObject.GetName(), e.Err).Error()
}

type StampError struct {
	Err      error
	Resource *v1alpha1.SupplyChainResource
}

func (e StampError) Error() string {
	return fmt.Errorf("unable to stamp object for resource [%s]: %w", e.Resource.Name, e.Err).Error()
}

type RetrieveOutputError struct {
	Err           error
	Resource      *v1alpha1.SupplyChainResource
	StampedObject *unstructured.Unstructured
}

func (e RetrieveOutputError) Error() string {
	return fmt.Errorf("unable to retrieve outputs [%s] from stamped object [%s/%s] of type [%s] for resource [%s]: %w",
		e.JsonPathExpression(), e.StampedObject.GetNamespace(), e.StampedObject.GetName(),
		utils.GetFullyQualifiedType(e.StampedObject),
		e.Resource.Name, e.Err).Error()
}

type JsonPathErrorContext interface {
	JsonPathExpression() string
}

func (e RetrieveOutputError) JsonPathExpression() string {
	jsonPathErrorContext, ok := e.Err.(JsonPathErrorContext)
	if ok {
		return jsonPathErrorContext.JsonPathExpression()
	}
	return "<no jsonpath context>"
}
