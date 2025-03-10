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

package registrar

//go:generate go run github.com/maxbrunsfeld/counterfeiter/v6 -generate

import (
	"context"
	"fmt"
	"reflect"

	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/vmware-tanzu/cartographer/pkg/apis/v1alpha1"
	"github.com/vmware-tanzu/cartographer/pkg/repository"
)

//counterfeiter:generate sigs.k8s.io/controller-runtime/pkg/client.Client

//counterfeiter:generate . Logger
type Logger interface {
	Error(err error, msg string, keysAndValues ...interface{})
}

type Mapper struct {
	Client client.Client
	// fixme We should accept the context, not the logger - then we get the right logger and so does the client
	Logger Logger
}

func (mapper *Mapper) TemplateToDeliverableRequests(template client.Object) []reconcile.Request {
	deliveries := mapper.templateToDeliveries(template)

	var requests []reconcile.Request
	for _, delivery := range deliveries {
		reqs := mapper.ClusterDeliveryToDeliverableRequests(&delivery)
		requests = append(requests, reqs...)
	}

	return requests
}

func (mapper *Mapper) TemplateToWorkloadRequests(template client.Object) []reconcile.Request {
	supplyChains := mapper.templateToSupplyChains(template)

	var requests []reconcile.Request
	for _, supplyChain := range supplyChains {
		reqs := mapper.ClusterSupplyChainToWorkloadRequests(&supplyChain)
		requests = append(requests, reqs...)
	}

	return requests
}

func (mapper *Mapper) templateToSupplyChains(template client.Object) []v1alpha1.ClusterSupplyChain {
	templateName := template.GetName()

	err := mapper.addGVK(template)
	if err != nil {
		mapper.Logger.Error(err, fmt.Sprintf("could not get GVK for template: %s", templateName))
		return nil
	}

	list := &v1alpha1.ClusterSupplyChainList{}

	err = mapper.Client.List(
		context.TODO(),
		list,
	)

	if err != nil {
		mapper.Logger.Error(err, "list ClusterSupplyChains")
		return nil
	}

	templateKind := template.GetObjectKind().GroupVersionKind().Kind

	var supplyChains []v1alpha1.ClusterSupplyChain
	for _, sc := range list.Items {
		for _, res := range sc.Spec.Resources {
			if res.TemplateRef.Kind == templateKind && res.TemplateRef.Name == templateName {
				supplyChains = append(supplyChains, sc)
			}
		}
	}
	return supplyChains
}

func (mapper *Mapper) ClusterSupplyChainToWorkloadRequests(object client.Object) []reconcile.Request {
	supplyChain, ok := object.(*v1alpha1.ClusterSupplyChain)
	if !ok {
		mapper.Logger.Error(nil, "cluster supply chain to workload requests: cast to ClusterSupplyChain failed")
		return nil
	}

	workloads, err := mapper.clusterSupplyChainToWorkloads(*supplyChain)
	if err != nil {
		mapper.Logger.Error(err, "cluster supply chain to workload requests")
		return nil
	}

	var requests []reconcile.Request
	for _, workload := range workloads {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      workload.Name,
				Namespace: workload.Namespace,
			},
		})
	}

	return requests
}

func (mapper *Mapper) clusterSupplyChainToWorkloads(sc v1alpha1.ClusterSupplyChain) ([]v1alpha1.Workload, error) {
	err := mapper.addGVK(&sc)
	if err != nil {
		mapper.Logger.Error(err, fmt.Sprintf("could not get GVK for supply chain: %s", sc.Name))
		return nil, err
	}

	scList := &v1alpha1.ClusterSupplyChainList{}
	err = mapper.Client.List(context.TODO(), scList)
	if err != nil {
		mapper.Logger.Error(err, "cluster supply chain to workloads: client list supply chains")
		return nil, err
	}

	var selectorGetters []repository.SelectorGetter
	for _, item := range scList.Items {
		item := item
		selectorGetters = append(selectorGetters, &item)
	}

	workloadList := &v1alpha1.WorkloadList{}
	err = mapper.Client.List(context.TODO(), workloadList,
		client.InNamespace(sc.Namespace),
		client.MatchingLabels(sc.Spec.Selector))
	if err != nil {
		mapper.Logger.Error(err, "cluster supply chain to workloads: client list workloads")
		return nil, err
	}

	var matchingWorkloads []v1alpha1.Workload
	for _, wl := range workloadList.Items {
		for _, matchingObject := range repository.BestLabelMatches(&wl, selectorGetters) {
			matchingSC := matchingObject.(*v1alpha1.ClusterSupplyChain)
			if reflect.DeepEqual(matchingSC, &sc) {
				matchingWorkloads = append(matchingWorkloads, wl)
			}
		}
	}

	return matchingWorkloads, nil
}

func (mapper *Mapper) ClusterDeliveryToDeliverableRequests(object client.Object) []reconcile.Request {
	var err error

	delivery, ok := object.(*v1alpha1.ClusterDelivery)
	if !ok {
		mapper.Logger.Error(nil, "cluster delivery to deliverable requests: cast to ClusterDelivery failed")
		return nil
	}

	deliverables, err := mapper.clusterDeliveryToDeliverables(*delivery)
	if err != nil {
		mapper.Logger.Error(err, "cluster delivery to deliverable requests")
		return nil
	}

	var requests []reconcile.Request
	for _, deliverable := range deliverables {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      deliverable.Name,
				Namespace: deliverable.Namespace,
			},
		})
	}

	return requests
}

func (mapper *Mapper) clusterDeliveryToDeliverables(d v1alpha1.ClusterDelivery) ([]v1alpha1.Deliverable, error) {
	err := mapper.addGVK(&d)
	if err != nil {
		mapper.Logger.Error(err, fmt.Sprintf("could not get GVK for delivery: %s", d.Name))
		return nil, err
	}

	deliveryList := &v1alpha1.ClusterDeliveryList{}
	err = mapper.Client.List(context.TODO(), deliveryList)
	if err != nil {
		mapper.Logger.Error(err, "cluster delivery to deliverables: client list deliveries")
		return nil, err
	}

	var selectorGetters []repository.SelectorGetter
	for _, item := range deliveryList.Items {
		item := item
		selectorGetters = append(selectorGetters, &item)
	}

	deliverableList := &v1alpha1.DeliverableList{}
	err = mapper.Client.List(context.TODO(), deliverableList,
		client.InNamespace(d.Namespace),
		client.MatchingLabels(d.Spec.Selector))
	if err != nil {
		mapper.Logger.Error(err, "cluster delivery to deliverables: client list deliverables")
		return nil, err
	}

	var matchingDeliverables []v1alpha1.Deliverable
	for _, deliverable := range deliverableList.Items {
		for _, matchingObject := range repository.BestLabelMatches(&deliverable, selectorGetters) {
			matchingDelivery := matchingObject.(*v1alpha1.ClusterDelivery)
			if reflect.DeepEqual(matchingDelivery, &d) {
				matchingDeliverables = append(matchingDeliverables, deliverable)
			}
		}
	}

	return matchingDeliverables, nil
}

func (mapper *Mapper) RunTemplateToRunnableRequests(object client.Object) []reconcile.Request {
	var err error

	runTemplate, ok := object.(*v1alpha1.ClusterRunTemplate)
	if !ok {
		mapper.Logger.Error(nil, "run template to runnable requests: cast to run template failed")
		return nil
	}

	list := &v1alpha1.RunnableList{}

	err = mapper.Client.List(context.TODO(), list)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "run template to runnable requests: client list")
		return nil
	}

	var requests []reconcile.Request
	for _, runnable := range list.Items {

		if runTemplateRefMatch(runnable.Spec.RunTemplateRef, runTemplate) {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      runnable.Name,
					Namespace: runnable.Namespace,
				},
			})
		}
	}

	return requests
}

// addGVK fulfills the 'GVK of an object returned from the APIServer
// https://github.com/kubernetes-sigs/controller-runtime/issues/1517#issuecomment-844703142
func (mapper *Mapper) addGVK(obj client.Object) error {
	gvks, unversioned, err := mapper.Client.Scheme().ObjectKinds(obj)
	if err != nil {
		return fmt.Errorf("missing apiVersion or kind: %s err: %w", obj.GetName(), err)
	}

	if unversioned {
		return fmt.Errorf("unversioned object: %s", obj.GetName())
	}

	if len(gvks) != 1 {
		return fmt.Errorf("unexpected GVK count: %s", obj.GetName())
	}

	obj.GetObjectKind().SetGroupVersionKind(gvks[0])
	return nil
}

func (mapper *Mapper) TemplateToSupplyChainRequests(template client.Object) []reconcile.Request {
	supplyChains := mapper.templateToSupplyChains(template)

	var requests []reconcile.Request
	for _, supplyChain := range supplyChains {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: supplyChain.Name,
			},
		})
	}

	return requests
}

func (mapper *Mapper) TemplateToDeliveryRequests(template client.Object) []reconcile.Request {
	deliveries := mapper.templateToDeliveries(template)

	var requests []reconcile.Request
	for _, delivery := range deliveries {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name: delivery.Name,
			},
		})
	}

	return requests
}

func (mapper *Mapper) templateToDeliveries(template client.Object) []v1alpha1.ClusterDelivery {
	templateName := template.GetName()

	err := mapper.addGVK(template)
	if err != nil {
		mapper.Logger.Error(err, fmt.Sprintf("could not get GVK for template: %s", templateName))
		return nil
	}

	list := &v1alpha1.ClusterDeliveryList{}

	err = mapper.Client.List(
		context.TODO(),
		list,
	)

	if err != nil {
		mapper.Logger.Error(err, "list ClusterDeliveries")
		return nil
	}

	templateKind := template.GetObjectKind().GroupVersionKind().Kind

	var deliveries []v1alpha1.ClusterDelivery
	for _, delivery := range list.Items {
		for _, res := range delivery.Spec.Resources {
			if res.TemplateRef.Kind == templateKind && res.TemplateRef.Name == templateName {
				deliveries = append(deliveries, delivery)
			}
		}
	}
	return deliveries
}

func runTemplateRefMatch(ref v1alpha1.TemplateReference, runTemplate *v1alpha1.ClusterRunTemplate) bool {
	if ref.Name != runTemplate.Name {
		return false
	}

	return ref.Kind == "ClusterRunTemplate" || ref.Kind == ""
}

func (mapper *Mapper) ServiceAccountToWorkloadRequests(serviceAccountObject client.Object) []reconcile.Request {
	list := &v1alpha1.WorkloadList{}

	err := mapper.Client.List(context.TODO(), list)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "service account to workload requests: list workloads")
		return nil
	}

	requestMap := make(map[reconcile.Request]bool)
	for _, workload := range list.Items {
		if workload.Namespace == serviceAccountObject.GetNamespace() && workload.Spec.ServiceAccountName == serviceAccountObject.GetName() {
			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      workload.Name,
					Namespace: workload.Namespace,
				},
			}
			requestMap[request] = true
		}
	}

	supplyChains := mapper.serviceAccountToSupplyChains(serviceAccountObject)
	for _, sc := range supplyChains {
		scWorkloads, err := mapper.clusterSupplyChainToWorkloads(sc)
		if err != nil {
			mapper.Logger.Error(err, "service account to workload requests")
			return nil
		}
		for _, workload := range scWorkloads {
			if workload.Spec.ServiceAccountName != "" {
				continue
			}

			if sc.Spec.ServiceAccountRef.Namespace == serviceAccountObject.GetNamespace() ||
				(sc.Spec.ServiceAccountRef.Namespace == "" && workload.Namespace == serviceAccountObject.GetNamespace()) {
				request := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      workload.Name,
						Namespace: workload.Namespace,
					},
				}
				requestMap[request] = true
			}
		}
	}

	var requests []reconcile.Request
	for r := range requestMap {
		requests = append(requests, r)
	}

	return requests
}

func (mapper *Mapper) serviceAccountToSupplyChains(serviceAccountObject client.Object) []v1alpha1.ClusterSupplyChain {
	list := &v1alpha1.ClusterSupplyChainList{}

	err := mapper.Client.List(context.TODO(), list)
	if err != nil {
		mapper.Logger.Error(err, "service account to supply chains: list supply chains")
		return nil
	}

	var supplyChains []v1alpha1.ClusterSupplyChain
	for _, sc := range list.Items {
		if sc.Spec.ServiceAccountRef.Name == serviceAccountObject.GetName() {
			supplyChains = append(supplyChains, sc)
		}
	}

	return supplyChains
}

func (mapper *Mapper) RoleBindingToWorkloadRequests(roleBindingObject client.Object) []reconcile.Request {
	roleBinding, ok := roleBindingObject.(*rbacv1.RoleBinding)
	if !ok {
		mapper.Logger.Error(nil, "role binding to workload requests: cast to RoleBinding failed")
		return nil
	}

	for _, subject := range roleBinding.Subjects {
		if subject.APIGroup == "" && subject.Kind == "ServiceAccount" {
			serviceAccountObject := &corev1.ServiceAccount{}
			serviceAccountKey := client.ObjectKey{
				Namespace: subject.Namespace,
				Name:      subject.Name,
			}
			err := mapper.Client.Get(context.TODO(), serviceAccountKey, serviceAccountObject)
			if err != nil {
				mapper.Logger.Error(fmt.Errorf("client get: %w", err), "role binding to workload requests: get service account")
			}
			return mapper.ServiceAccountToWorkloadRequests(serviceAccountObject)
		}
	}

	return []reconcile.Request{}
}

func (mapper *Mapper) ClusterRoleBindingToWorkloadRequests(clusterRoleBindingObject client.Object) []reconcile.Request {
	clusterRoleBinding, ok := clusterRoleBindingObject.(*rbacv1.ClusterRoleBinding)
	if !ok {
		mapper.Logger.Error(nil, "cluster role binding to workload requests: cast to ClusterRoleBinding failed")
		return nil
	}

	for _, subject := range clusterRoleBinding.Subjects {
		if subject.APIGroup == "" && subject.Kind == "ServiceAccount" {
			serviceAccountObject := &corev1.ServiceAccount{}
			serviceAccountKey := client.ObjectKey{
				Namespace: subject.Namespace,
				Name:      subject.Name,
			}
			err := mapper.Client.Get(context.TODO(), serviceAccountKey, serviceAccountObject)
			if err != nil {
				mapper.Logger.Error(fmt.Errorf("client get: %w", err), "cluster role binding to workload requests: get service account")
				return []reconcile.Request{}
			}
			return mapper.ServiceAccountToWorkloadRequests(serviceAccountObject)
		}
	}

	return []reconcile.Request{}
}

func (mapper *Mapper) RoleToWorkloadRequests(roleObject client.Object) []reconcile.Request {
	role, ok := roleObject.(*rbacv1.Role)
	if !ok {
		mapper.Logger.Error(nil, "role to workload requests: cast to Role failed")
		return nil
	}

	list := &rbacv1.RoleBindingList{}

	err := mapper.Client.List(context.TODO(), list)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "role to workload requests: list role bindings")
		return nil
	}

	var requests []reconcile.Request
	for _, roleBinding := range list.Items {
		if roleBinding.RoleRef.APIGroup == "" && roleBinding.RoleRef.Kind == "Role" && roleBinding.RoleRef.Name == role.Name && roleBinding.Namespace == role.Namespace {
			requests = append(requests, mapper.RoleBindingToWorkloadRequests(&roleBinding)...)
		}
	}

	return requests
}

func (mapper *Mapper) ClusterRoleToWorkloadRequests(clusterRoleObject client.Object) []reconcile.Request {
	clusterRole, ok := clusterRoleObject.(*rbacv1.ClusterRole)
	if !ok {
		mapper.Logger.Error(nil, "cluster role to workload requests: cast to ClusterRole failed")
		return nil
	}

	clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}

	err := mapper.Client.List(context.TODO(), clusterRoleBindingList)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "cluster role to workload requests: list cluster role bindings")
		return nil
	}

	var requests []reconcile.Request

	for _, clusterRoleBinding := range clusterRoleBindingList.Items {
		if clusterRoleBinding.RoleRef.APIGroup == "" && clusterRoleBinding.RoleRef.Kind == "ClusterRole" && clusterRoleBinding.RoleRef.Name == clusterRole.Name {
			requests = append(requests, mapper.ClusterRoleBindingToWorkloadRequests(&clusterRoleBinding)...)
		}
	}

	roleBindingList := &rbacv1.RoleBindingList{}

	err = mapper.Client.List(context.TODO(), roleBindingList)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "cluster role role to workload requests: list role bindings")
		return nil
	}

	for _, roleBinding := range roleBindingList.Items {
		if roleBinding.RoleRef.APIGroup == "" && roleBinding.RoleRef.Kind == "ClusterRole" && roleBinding.RoleRef.Name == clusterRole.Name {
			requests = append(requests, mapper.RoleBindingToWorkloadRequests(&roleBinding)...)
		}
	}

	return requests
}

func (mapper *Mapper) ServiceAccountToDeliverableRequests(serviceAccountObject client.Object) []reconcile.Request {
	list := &v1alpha1.DeliverableList{}

	err := mapper.Client.List(context.TODO(), list)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "service account to deliverable requests: list deliverables")
		return nil
	}

	requestMap := make(map[reconcile.Request]bool)
	for _, deliverable := range list.Items {
		if deliverable.Namespace == serviceAccountObject.GetNamespace() && deliverable.Spec.ServiceAccountName == serviceAccountObject.GetName() {
			request := reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      deliverable.Name,
					Namespace: deliverable.Namespace,
				},
			}
			requestMap[request] = true
		}
	}

	deliveries := mapper.serviceAccountToDeliveries(serviceAccountObject)
	for _, d := range deliveries {
		deliveryDeliverables, err := mapper.clusterDeliveryToDeliverables(d)
		if err != nil {
			mapper.Logger.Error(err, "service account to deliverable requests")
		}
		for _, deliverable := range deliveryDeliverables {
			if deliverable.Spec.ServiceAccountName != "" {
				continue
			}

			if d.Spec.ServiceAccountRef.Namespace == serviceAccountObject.GetNamespace() ||
				(d.Spec.ServiceAccountRef.Namespace == "" && deliverable.Namespace == serviceAccountObject.GetNamespace()) {
				request := reconcile.Request{
					NamespacedName: types.NamespacedName{
						Name:      deliverable.Name,
						Namespace: deliverable.Namespace,
					},
				}
				requestMap[request] = true
			}
		}
	}

	var requests []reconcile.Request
	for r := range requestMap {
		requests = append(requests, r)
	}

	return requests
}

func (mapper *Mapper) serviceAccountToDeliveries(serviceAccountObject client.Object) []v1alpha1.ClusterDelivery {
	list := &v1alpha1.ClusterDeliveryList{}

	err := mapper.Client.List(context.TODO(), list)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "service account to deliveries: list deliveries")
		return nil
	}

	var deliveries []v1alpha1.ClusterDelivery
	for _, d := range list.Items {
		if d.Spec.ServiceAccountRef.Name == serviceAccountObject.GetName() {
			deliveries = append(deliveries, d)
		}
	}

	return deliveries
}

func (mapper *Mapper) RoleBindingToDeliverableRequests(roleBindingObject client.Object) []reconcile.Request {
	roleBinding, ok := roleBindingObject.(*rbacv1.RoleBinding)
	if !ok {
		mapper.Logger.Error(nil, "role binding to deliverable requests: cast to RoleBinding failed")
		return nil
	}

	for _, subject := range roleBinding.Subjects {
		if subject.APIGroup == "" && subject.Kind == "ServiceAccount" {
			serviceAccountObject := &corev1.ServiceAccount{}
			serviceAccountKey := client.ObjectKey{
				Namespace: subject.Namespace,
				Name:      subject.Name,
			}
			err := mapper.Client.Get(context.TODO(), serviceAccountKey, serviceAccountObject)
			if err != nil {
				mapper.Logger.Error(fmt.Errorf("client get: %w", err), "role binding to deliverable requests: get service account")
			}
			return mapper.ServiceAccountToDeliverableRequests(serviceAccountObject)
		}
	}

	return []reconcile.Request{}
}

func (mapper *Mapper) ClusterRoleBindingToDeliverableRequests(clusterRoleBindingObject client.Object) []reconcile.Request {
	clusterRoleBinding, ok := clusterRoleBindingObject.(*rbacv1.ClusterRoleBinding)
	if !ok {
		mapper.Logger.Error(nil, "cluster role binding to deliverable requests: cast to ClusterRoleBinding failed")
		return nil
	}

	for _, subject := range clusterRoleBinding.Subjects {
		if subject.APIGroup == "" && subject.Kind == "ServiceAccount" {
			serviceAccountObject := &corev1.ServiceAccount{}
			serviceAccountKey := client.ObjectKey{
				Namespace: subject.Namespace,
				Name:      subject.Name,
			}
			err := mapper.Client.Get(context.TODO(), serviceAccountKey, serviceAccountObject)
			if err != nil {
				mapper.Logger.Error(fmt.Errorf("client get: %w", err), "cluster role binding to deliverable requests: get service account")
				return []reconcile.Request{}
			}
			return mapper.ServiceAccountToDeliverableRequests(serviceAccountObject)
		}
	}

	return []reconcile.Request{}
}

func (mapper *Mapper) RoleToDeliverableRequests(roleObject client.Object) []reconcile.Request {
	role, ok := roleObject.(*rbacv1.Role)
	if !ok {
		mapper.Logger.Error(nil, "role to deliverable requests: cast to Role failed")
		return nil
	}

	list := &rbacv1.RoleBindingList{}

	err := mapper.Client.List(context.TODO(), list)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "role to deliverable requests: list role bindings")
		return nil
	}

	var requests []reconcile.Request
	for _, roleBinding := range list.Items {
		if roleBinding.RoleRef.APIGroup == "" && roleBinding.RoleRef.Kind == "Role" && roleBinding.RoleRef.Name == role.Name && roleBinding.Namespace == role.Namespace {
			requests = append(requests, mapper.RoleBindingToDeliverableRequests(&roleBinding)...)
		}
	}

	return requests
}

func (mapper *Mapper) ClusterRoleToDeliverableRequests(clusterRoleObject client.Object) []reconcile.Request {
	clusterRole, ok := clusterRoleObject.(*rbacv1.ClusterRole)
	if !ok {
		mapper.Logger.Error(nil, "cluster role to deliverable requests: cast to ClusterRole failed")
		return nil
	}

	clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}

	err := mapper.Client.List(context.TODO(), clusterRoleBindingList)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "cluster role to deliverable requests: list cluster role bindings")
		return nil
	}

	var requests []reconcile.Request

	for _, clusterRoleBinding := range clusterRoleBindingList.Items {
		if clusterRoleBinding.RoleRef.APIGroup == "" && clusterRoleBinding.RoleRef.Kind == "ClusterRole" && clusterRoleBinding.RoleRef.Name == clusterRole.Name {
			requests = append(requests, mapper.ClusterRoleBindingToDeliverableRequests(&clusterRoleBinding)...)
		}
	}

	roleBindingList := &rbacv1.RoleBindingList{}

	err = mapper.Client.List(context.TODO(), roleBindingList)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "cluster role role to deliverable requests: list role bindings")
		return nil
	}

	for _, roleBinding := range roleBindingList.Items {
		if roleBinding.RoleRef.APIGroup == "" && roleBinding.RoleRef.Kind == "ClusterRole" && roleBinding.RoleRef.Name == clusterRole.Name {
			requests = append(requests, mapper.RoleBindingToDeliverableRequests(&roleBinding)...)
		}
	}

	return requests
}

func (mapper *Mapper) ServiceAccountToRunnableRequests(serviceAccountObject client.Object) []reconcile.Request {
	list := &v1alpha1.RunnableList{}

	err := mapper.Client.List(context.TODO(), list)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "service account to runnable requests: list runnables")
		return nil
	}

	var requests []reconcile.Request
	for _, runnable := range list.Items {
		if runnable.Namespace == serviceAccountObject.GetNamespace() && runnable.Spec.ServiceAccountName == serviceAccountObject.GetName() {
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      runnable.Name,
					Namespace: runnable.Namespace,
				},
			})
		}
	}

	return requests
}

func (mapper *Mapper) RoleBindingToRunnableRequests(roleBindingObject client.Object) []reconcile.Request {
	roleBinding, ok := roleBindingObject.(*rbacv1.RoleBinding)
	if !ok {
		mapper.Logger.Error(nil, "role binding to runnable requests: cast to RoleBinding failed")
		return nil
	}

	for _, subject := range roleBinding.Subjects {
		if subject.APIGroup == "" && subject.Kind == "ServiceAccount" {
			serviceAccountObject := &corev1.ServiceAccount{}

			serviceAccountKey := client.ObjectKey{
				Namespace: subject.Namespace,
				Name:      subject.Name,
			}
			err := mapper.Client.Get(context.TODO(), serviceAccountKey, serviceAccountObject)
			if err != nil {
				mapper.Logger.Error(fmt.Errorf("client get: %w", err), "role binding to runnable requests: get service account")
			}
			return mapper.ServiceAccountToRunnableRequests(serviceAccountObject)
		}
	}

	return []reconcile.Request{}
}

func (mapper *Mapper) ClusterRoleBindingToRunnableRequests(clusterRoleBindingObject client.Object) []reconcile.Request {
	clusterRoleBinding, ok := clusterRoleBindingObject.(*rbacv1.ClusterRoleBinding)
	if !ok {
		mapper.Logger.Error(nil, "cluster role binding to runnable requests: cast to ClusterRoleBinding failed")
		return nil
	}

	for _, subject := range clusterRoleBinding.Subjects {
		if subject.APIGroup == "" && subject.Kind == "ServiceAccount" {
			serviceAccountObject := &corev1.ServiceAccount{}
			serviceAccountKey := client.ObjectKey{
				Namespace: subject.Namespace,
				Name:      subject.Name,
			}
			err := mapper.Client.Get(context.TODO(), serviceAccountKey, serviceAccountObject)
			if err != nil {
				mapper.Logger.Error(fmt.Errorf("client get: %w", err), "cluster role binding to runnable requests: get service account")
				return []reconcile.Request{}
			}
			return mapper.ServiceAccountToRunnableRequests(serviceAccountObject)
		}
	}

	return []reconcile.Request{}
}

func (mapper *Mapper) RoleToRunnableRequests(roleObject client.Object) []reconcile.Request {
	role, ok := roleObject.(*rbacv1.Role)
	if !ok {
		mapper.Logger.Error(nil, "role to runnable requests: cast to Role failed")
		return nil
	}

	list := &rbacv1.RoleBindingList{}

	err := mapper.Client.List(context.TODO(), list)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "role to runnable requests: list role bindings")
		return nil
	}

	var requests []reconcile.Request
	for _, roleBinding := range list.Items {
		if roleBinding.RoleRef.APIGroup == "" && roleBinding.RoleRef.Kind == "Role" && roleBinding.RoleRef.Name == role.Name && roleBinding.Namespace == role.Namespace {
			requests = append(requests, mapper.RoleBindingToRunnableRequests(&roleBinding)...)
		}
	}

	return requests
}

func (mapper *Mapper) ClusterRoleToRunnableRequests(clusterRoleObject client.Object) []reconcile.Request {
	clusterRole, ok := clusterRoleObject.(*rbacv1.ClusterRole)
	if !ok {
		mapper.Logger.Error(nil, "cluster role to runnable requests: cast to ClusterRole failed")
		return nil
	}

	clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}

	err := mapper.Client.List(context.TODO(), clusterRoleBindingList)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "cluster role to runnable requests: list cluster role bindings")
		return nil
	}

	var requests []reconcile.Request

	for _, clusterRoleBinding := range clusterRoleBindingList.Items {
		if clusterRoleBinding.RoleRef.APIGroup == "" && clusterRoleBinding.RoleRef.Kind == "ClusterRole" && clusterRoleBinding.RoleRef.Name == clusterRole.Name {
			requests = append(requests, mapper.ClusterRoleBindingToRunnableRequests(&clusterRoleBinding)...)
		}
	}

	roleBindingList := &rbacv1.RoleBindingList{}

	err = mapper.Client.List(context.TODO(), roleBindingList)
	if err != nil {
		mapper.Logger.Error(fmt.Errorf("client list: %w", err), "cluster role role to runnable requests: list role bindings")
		return nil
	}

	for _, roleBinding := range roleBindingList.Items {
		if roleBinding.RoleRef.APIGroup == "" && roleBinding.RoleRef.Kind == "ClusterRole" && roleBinding.RoleRef.Name == clusterRole.Name {
			requests = append(requests, mapper.RoleBindingToRunnableRequests(&roleBinding)...)
		}
	}

	return requests
}
