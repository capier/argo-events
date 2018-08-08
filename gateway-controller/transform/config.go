package transform

import (
	"context"
	"fmt"
	"github.com/argoproj/argo-events/common"
	apiv1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/watch"
	"k8s.io/client-go/tools/cache"
)

func (e *eOperationCtx) WatchEventConfigMap(ctx context.Context, name string) (cache.Controller, error) {
	source := e.newStoreConfigMapWatch(name)
	_, controller := cache.NewInformer(
		source,
		&apiv1.ConfigMap{},
		0,
		cache.ResourceEventHandlerFuncs{
			AddFunc: func(obj interface{}) {
				if cm, ok := obj.(*apiv1.ConfigMap); ok {
					e.log.Info().Str("config-map", name).Msg("detected ConfigMap update. Updating the controller config.")
					err := e.updateConfig(cm)
					if err != nil {
						e.log.Error().Err(err).Msg("update of config failed")
					}
				}
			},
			UpdateFunc: func(old, new interface{}) {
				if newCm, ok := new.(*apiv1.ConfigMap); ok {
					e.log.Info().Msg("detected ConfigMap update. Updating the controller config.")
					err := e.updateConfig(newCm)
					if err != nil {
						e.log.Error().Err(err).Msg("update of config failed")
					}
				}
			},
		})

	go controller.Run(ctx.Done())
	return controller, nil
}

func (e *eOperationCtx) newStoreConfigMapWatch(name string) *cache.ListWatch {
	x := e.kubeClientset.CoreV1().RESTClient()
	resource := "configmaps"
	fieldSelector := fields.ParseSelectorOrDie(fmt.Sprintf("metadata.name=%s", name))

	listFunc := func(options metav1.ListOptions) (runtime.Object, error) {
		options.FieldSelector = fieldSelector.String()
		req := x.Get().
			Namespace(e.Namespace).
			Resource(resource).
			VersionedParams(&options, metav1.ParameterCodec)
		return req.Do().Get()
	}
	watchFunc := func(options metav1.ListOptions) (watch.Interface, error) {
		options.Watch = true
		options.FieldSelector = fieldSelector.String()
		req := x.Get().
			Namespace(e.Namespace).
			Resource(resource).
			VersionedParams(&options, metav1.ParameterCodec)
		return req.Watch()
	}
	return &cache.ListWatch{ListFunc: listFunc, WatchFunc: watchFunc}
}

func (e *eOperationCtx) updateConfig(cm *apiv1.ConfigMap) error {
	eventType, ok := cm.Data[common.EventType]
	if !ok {
		return fmt.Errorf("configMap '%s' does not have key '%s'", cm.Name, common.EventType)
	}
	eventTypeVersion, ok := cm.Data[common.EventTypeVersion]
	if !ok {
		return fmt.Errorf("configMap '%s' does not have key '%s'", cm.Name, common.EventTypeVersion)
	}
	e.Config = EventConfig{
		EventType:        eventType,
		EventTypeVersion: eventTypeVersion,
	}
	return nil
}
