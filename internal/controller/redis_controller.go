/*
Copyright 2025.

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

package controller

import (
	"context"
	"fmt"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	logf "sigs.k8s.io/controller-runtime/pkg/log"

	databasev1alpha1 "github.com/KateScarlet/redis-operator/api/v1alpha1"
	"github.com/KateScarlet/redis-operator/internal/resources"
	"github.com/go-logr/logr"
	kruisev1alpha1 "github.com/openkruise/kruise-api/apps/v1alpha1"
	kruisev1beta1 "github.com/openkruise/kruise-api/apps/v1beta1"
)

const (
	typeAvailableRedis = "Available"
)

// RedisReconciler reconciles a Redis object
type RedisReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=database.phosphorlee.xyz,resources=redises,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=database.phosphorlee.xyz,resources=redises/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=database.phosphorlee.xyz,resources=redises/finalizers,verbs=update
// +kubebuilder:rbac:groups=core,resources=events,verbs=create;patch
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods,verbs=get;list;watch

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// It is essential for the controller's reconciliation loop to be idempotent. By following the Operator
// pattern you will create Controllers which provide a reconcile function
// responsible for synchronizing resources until the desired state is reached on the cluster.
// Breaking this recommendation goes against the design principles of controller-runtime.
// and may lead to unforeseen consequences such as resources becoming stuck and requiring manual intervention.
// For further info:
// - About Operator Pattern: https://kubernetes.io/docs/concepts/extend-kubernetes/operator/
// - About Controllers: https://kubernetes.io/docs/concepts/architecture/controller/
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.21.0/pkg/reconcile
func (r *RedisReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := logf.FromContext(ctx)
	redis := &databasev1alpha1.Redis{}
	err := r.Get(ctx, req.NamespacedName, redis)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("redis resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get redis")
		return ctrl.Result{}, err
	}

	if len(redis.Status.Conditions) == 0 {
		meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: typeAvailableRedis, Status: metav1.ConditionUnknown, Reason: "Reconciling", Message: "Starting reconciliation"})
		if err = r.Status().Update(ctx, redis); err != nil {
			log.Error(err, "Failed to update Redis status")
			return ctrl.Result{}, err
		}

		if err := r.Get(ctx, req.NamespacedName, redis); err != nil {
			log.Error(err, "Failed to re-fetch redis")
			return ctrl.Result{}, err
		}
	}

	res, err := r.reconcileRedisService(ctx, redis, log)
	if err != nil {
		return res, err
	}

	res, err = r.reconcileRedisConfigMap(ctx, redis, log)
	if err != nil {
		return res, err
	}

	res, err = r.reconcileRedisPodProbeMarker(ctx, redis, log)
	if err != nil {
		return res, err
	}

	res, err = r.reconcileRedisAdvancedStatefulSet(ctx, redis, log, req)
	if err != nil {
		return res, err
	}

	res, err = r.reconcileRedisSentinelClusterIPService(ctx, redis, log)
	if err != nil {
		return res, err
	}

	res, err = r.reconcileRedisSentinelConfigMap(ctx, redis, log)
	if err != nil {
		return res, err
	}

	res, err = r.reconcileRedisSentinelDeployment(ctx, redis, log)
	if err != nil {
		return res, err
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *RedisReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&databasev1alpha1.Redis{}).
		Owns(&kruisev1beta1.StatefulSet{}).
		Owns(&kruisev1alpha1.PodProbeMarker{}).
		Owns(&corev1.ConfigMap{}).
		Owns(&corev1.Service{}).
		Named("redis").
		Complete(r)
}

func (r *RedisReconciler) reconcileRedisService(ctx context.Context, redis *databasev1alpha1.Redis, log logr.Logger) (ctrl.Result, error) {
	foundSvc := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: redis.Name + "-headless", Namespace: redis.Namespace}, foundSvc)
	if err != nil && apierrors.IsNotFound(err) {
		svc, err := resources.HeadlessServiceForRedis(redis, r.Scheme)
		if err != nil {
			log.Error(err, "Failed to define new ConfigMap resource for Redis")

			// The following implementation will update the status
			meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: typeAvailableRedis,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create HeadlessService for the custom resource (%s): (%s)", redis.Name, err)})

			if err := r.Status().Update(ctx, redis); err != nil {
				log.Error(err, "Failed to update Redis status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}

		log.Info("Creating a new HeadlessService",
			"HeadlessService.Namespace", svc.Namespace, "HeadlessService.Name", svc.Name)
		if err = r.Create(ctx, svc); err != nil {
			log.Error(err, "Failed to create new HeadlessService",
				"HeadlessService.Namespace", svc.Namespace, "HeadlessService.Name", svc.Name)
			return ctrl.Result{}, err
		}

		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get HeadlessService")
		// Let's return the error for the reconciliation be re-trigged again
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *RedisReconciler) reconcileRedisConfigMap(ctx context.Context, redis *databasev1alpha1.Redis, log logr.Logger) (ctrl.Result, error) {
	foundCm := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: redis.Name + "-config", Namespace: redis.Namespace}, foundCm)
	if err != nil && apierrors.IsNotFound(err) {
		sts, err := resources.ConfigMapForRedis(redis, r.Scheme)
		if err != nil {
			log.Error(err, "Failed to define new ConfigMap resource for Redis")
			meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: typeAvailableRedis,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create ConfigMap for the custom resource (%s): (%s)", redis.Name, err)})
			if err := r.Status().Update(ctx, redis); err != nil {
				log.Error(err, "Failed to update Redis status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		log.Info("Creating a new ConfigMap",
			"ConfigMap.Namespace", sts.Namespace, "ConfigMap.Name", sts.Name)
		if err = r.Create(ctx, sts); err != nil {
			log.Error(err, "Failed to create new ConfigMap",
				"ConfigMap.Namespace", sts.Namespace, "ConfigMap.Name", sts.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get ConfigMap")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *RedisReconciler) reconcileRedisAdvancedStatefulSet(ctx context.Context, redis *databasev1alpha1.Redis, log logr.Logger, req ctrl.Request) (ctrl.Result, error) {
	foundSts := &kruisev1beta1.StatefulSet{}
	err := r.Get(ctx, types.NamespacedName{Name: redis.Name, Namespace: redis.Namespace}, foundSts)
	if err != nil && apierrors.IsNotFound(err) {
		sts, err := resources.AdvancedStatefulSetForRedis(redis, r.Scheme)
		if err != nil {
			log.Error(err, "Failed to define new StatefulSet resource for Redis")
			meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: typeAvailableRedis,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create StatefulSet for the custom resource (%s): (%s)", redis.Name, err)})
			if err := r.Status().Update(ctx, redis); err != nil {
				log.Error(err, "Failed to update Redis status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		log.Info("Creating a new StatefulSet",
			"StatefulSet.Namespace", sts.Namespace, "StatefulSet.Name", sts.Name)
		if err = r.Create(ctx, sts); err != nil {
			log.Error(err, "Failed to create new StatefulSet",
				"StatefulSet.Namespace", sts.Namespace, "StatefulSet.Name", sts.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get StatefulSet")
		return ctrl.Result{}, err
	}

	if *foundSts.Spec.Replicas != redis.Spec.Replicas {
		*foundSts.Spec.Replicas = redis.Spec.Replicas
		if err = r.Update(ctx, foundSts); err != nil {
			log.Error(err, "Failed to update StatefulSet",
				"StatefulSet.Namespace", foundSts.Namespace, "StatefulSet.Name", foundSts.Name)
			if err := r.Get(ctx, req.NamespacedName, redis); err != nil {
				log.Error(err, "Failed to re-fetch redis")
				return ctrl.Result{}, err
			}
			meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: typeAvailableRedis,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", redis.Name, err)})

			if err := r.Status().Update(ctx, redis); err != nil {
				log.Error(err, "Failed to update redis status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	if redis.Spec.Exporter.Enabled {
		if len(foundSts.Spec.Template.Spec.Containers) < 2 {
			foundSts.Spec.Template.Spec.Containers = append(foundSts.Spec.Template.Spec.Containers, corev1.Container{
				Image:           redis.Spec.Exporter.Image,
				Name:            "redis-exporter",
				ImagePullPolicy: corev1.PullIfNotPresent,
				Ports: []corev1.ContainerPort{{
					ContainerPort: 9121,
					Name:          "metrics",
				}},
				Env: []corev1.EnvVar{{
					Name:  "REDIS_ADDR",
					Value: "redis://localhost:6379",
				}},
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("100m"),
						corev1.ResourceMemory: resource.MustParse("128Mi"),
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("500m"),
						corev1.ResourceMemory: resource.MustParse("1024Mi"),
					},
				},
			})
			if err = r.Update(ctx, foundSts); err != nil {
				log.Error(err, "Failed to update StatefulSet with exporter container",
					"StatefulSet.Namespace", foundSts.Namespace, "StatefulSet.Name", foundSts.Name)
				if err := r.Get(ctx, req.NamespacedName, redis); err != nil {
					log.Error(err, "Failed to re-fetch redis")
					return ctrl.Result{}, err
				}
				meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: typeAvailableRedis,
					Status: metav1.ConditionFalse, Reason: "Resizing",
					Message: fmt.Sprintf("Failed to update the for the custom resource (%s): (%s)", redis.Name, err)})
				if err := r.Status().Update(ctx, redis); err != nil {
					log.Error(err, "Failed to update redis status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, err
			}
		}
	} else if len(foundSts.Spec.Template.Spec.Containers) > 1 {
		foundSts.Spec.Template.Spec.Containers = foundSts.Spec.Template.Spec.Containers[:1]
		if err = r.Update(ctx, foundSts); err != nil {
			log.Error(err, "Failed to update StatefulSet to remove exporter container",
				"StatefulSet.Namespace", foundSts.Namespace, "StatefulSet.Name", foundSts.Name)
			if err := r.Get(ctx, req.NamespacedName, redis); err != nil {
				log.Error(err, "Failed to re-fetch redis")
				return ctrl.Result{}, err
			}

			meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: typeAvailableRedis,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the for the custom resource (%s): (%s)", redis.Name, err)})

			if err := r.Status().Update(ctx, redis); err != nil {
				log.Error(err, "Failed to update redis status")
				return ctrl.Result{}, err
			}

			return ctrl.Result{}, err
		}
	}

	if foundSts.Spec.Template.Spec.Containers[0].Image != redis.Spec.Image {
		foundSts.Spec.Template.Spec.Containers[0].Image = redis.Spec.Image
		if err = r.Update(ctx, foundSts); err != nil {
			log.Error(err, "Failed to update StatefulSet",
				"StatefulSet.Namespace", foundSts.Namespace, "StatefulSet.Name", foundSts.Name)
			if err := r.Get(ctx, req.NamespacedName, redis); err != nil {
				log.Error(err, "Failed to re-fetch redis")
				return ctrl.Result{}, err
			}
			meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: typeAvailableRedis,
				Status: metav1.ConditionFalse, Reason: "Resizing",
				Message: fmt.Sprintf("Failed to update the size for the custom resource (%s): (%s)", redis.Name, err)})

			if err := r.Status().Update(ctx, redis); err != nil {
				log.Error(err, "Failed to update redis status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: typeAvailableRedis,
		Status: metav1.ConditionTrue, Reason: "Reconciling",
		Message: fmt.Sprintf("StatefulSet for custom resource (%s) created successfully", redis.Name)})

	if err := r.Status().Update(ctx, redis); err != nil {
		log.Error(err, "Failed to update Redis status")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

func (r *RedisReconciler) reconcileRedisSentinelDeployment(ctx context.Context, redis *databasev1alpha1.Redis, log logr.Logger) (ctrl.Result, error) {
	foundDep := &appsv1.Deployment{}
	if redis.Spec.Sentinel.Enabled {
		err := r.Get(ctx, types.NamespacedName{Name: redis.Name + "-sentinel", Namespace: redis.Namespace}, foundDep)
		if err != nil && apierrors.IsNotFound(err) {
			dep, err := resources.DeploymentForRedisSentinel(redis, r.Scheme)
			if err != nil {
				log.Error(err, "Failed to define new Redis Sentinel Deployment resource")
				meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: typeAvailableRedis,
					Status: metav1.ConditionFalse, Reason: "Reconciling",
					Message: fmt.Sprintf("Failed to create Redis Sentinel Deployment for the custom resource (%s): (%s)", redis.Name, err)})
				if err := r.Status().Update(ctx, redis); err != nil {
					log.Error(err, "Failed to update Redis status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, err
			}
			log.Info("Creating a new Redis Sentinel Deployment",
				"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
			if err = r.Create(ctx, dep); err != nil {
				log.Error(err, "Failed to create new Redis Sentinel Deployment",
					"Deployment.Namespace", dep.Namespace, "Deployment.Name", dep.Name)
				meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: typeAvailableRedis,
					Status: metav1.ConditionFalse, Reason: "Reconciling",
					Message: fmt.Sprintf("Failed to create Redis Sentinel Deployment for the custom resource (%s): (%s)", redis.Name, err)})
				if err := r.Status().Update(ctx, redis); err != nil {
					log.Error(err, "Failed to update Redis status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, nil
		}
		if redis.Spec.Sentinel.Replicas != *foundDep.Spec.Replicas {
			foundDep.Spec.Replicas = &redis.Spec.Sentinel.Replicas
			if err = r.Update(ctx, foundDep); err != nil {
				log.Error(err, "Failed to update Redis Sentinel Deployment",
					"Deployment.Namespace", foundDep.Namespace, "Deployment.Name", foundDep.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
	} else {
		err := r.Get(ctx, types.NamespacedName{Name: redis.Name + "-sentinel", Namespace: redis.Namespace}, foundDep)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("Redis Sentinel Deployment not found, nothing to delete")
				return ctrl.Result{}, nil
			} else {
				return ctrl.Result{}, err
			}
		} else {
			err = r.Delete(ctx, foundDep)
			if err != nil {
				log.Error(err, "Failed to delete Redis Sentinel Deployment",
					"Deployment.Namespace", foundDep.Namespace, "Deployment.Name", foundDep.Name)
				return ctrl.Result{}, err
			}
			log.Info("Deleted Redis Sentinel Deployment",
				"Deployment.Namespace", foundDep.Namespace, "Deployment.Name", foundDep.Name)
			return ctrl.Result{}, nil
		}
	}
	return ctrl.Result{}, nil
}

func (r *RedisReconciler) reconcileRedisSentinelConfigMap(ctx context.Context, redis *databasev1alpha1.Redis, log logr.Logger) (ctrl.Result, error) {
	foundCm := &corev1.ConfigMap{}
	if redis.Spec.Sentinel.Enabled {
		err := r.Get(ctx, types.NamespacedName{Name: redis.Name + "-sentinel-config", Namespace: redis.Namespace}, foundCm)
		if err != nil && apierrors.IsNotFound(err) {
			cm, err := resources.ConfigMapForRedisSentinel(redis, r.Scheme)
			if err != nil {
				log.Error(err, "Failed to define new ConfigMap resource for Redis Sentinel")
				meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: typeAvailableRedis,
					Status: metav1.ConditionFalse, Reason: "Reconciling",
					Message: fmt.Sprintf("Failed to create ConfigMap for the custom resource (%s): (%s)", redis.Name, err)})
				if err := r.Status().Update(ctx, redis); err != nil {
					log.Error(err, "Failed to update Redis status")
					return ctrl.Result{}, err
				}
				return ctrl.Result{}, err
			}
			log.Info("Creating a new ConfigMap for Redis Sentinel",
				"ConfigMap.Namespace", cm.Namespace, "ConfigMap.Name", cm.Name)
			if err = r.Create(ctx, cm); err != nil {
				log.Error(err, "Failed to create new ConfigMap for Redis Sentinel",
					"ConfigMap.Namespace", cm.Namespace, "ConfigMap.Name", cm.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		} else if err != nil {
			log.Error(err, "Failed to get ConfigMap for Redis Sentinel")
			return ctrl.Result{}, err
		}

	} else {
		err := r.Get(ctx, types.NamespacedName{Name: redis.Name + "-sentinel-config", Namespace: redis.Namespace}, foundCm)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("Redis Sentinel ConfigMap not found, nothing to delete")
				return ctrl.Result{}, nil
			} else {
				return ctrl.Result{}, err
			}
		} else {
			err = r.Delete(ctx, foundCm)
			if err != nil {
				log.Error(err, "Failed to delete Redis Sentinel ConfigMap",
					"ConfigMap.Namespace", foundCm.Namespace, "ConfigMap.Name", foundCm.Name)
				return ctrl.Result{}, err
			}
			log.Info("Deleted Redis Sentinel ConfigMap",
				"ConfigMap.Namespace", foundCm.Namespace, "ConfigMap.Name", foundCm.Name)
			return ctrl.Result{}, nil
		}
	}
	return ctrl.Result{}, nil
}

func (r *RedisReconciler) reconcileRedisSentinelClusterIPService(ctx context.Context, redis *databasev1alpha1.Redis, log logr.Logger) (ctrl.Result, error) {
	foundSvc := &corev1.Service{}
	if redis.Spec.Sentinel.Enabled {
		err := r.Get(ctx, types.NamespacedName{Name: redis.Name + "-sentinel", Namespace: redis.Namespace}, foundSvc)
		if err != nil && apierrors.IsNotFound(err) {
			svc, err := resources.ClusterIPServiceForRedisSentinel(redis, r.Scheme)
			if err != nil {
				log.Error(err, "Failed to define new ClusterIP Service resource for Redis Sentinel")
				return ctrl.Result{}, err
			}
			log.Info("Creating a new ClusterIP Service for Redis Sentinel",
				"Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
			if err = r.Create(ctx, svc); err != nil {
				log.Error(err, "Failed to create new ClusterIP Service for Redis Sentinel",
					"Service.Namespace", svc.Namespace, "Service.Name", svc.Name)
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: time.Minute}, nil
		} else if err != nil {
			log.Error(err, "Failed to get ClusterIP Service for Redis Sentinel")
			return ctrl.Result{}, err
		}
	} else {
		err := r.Get(ctx, types.NamespacedName{Name: redis.Name + "-sentinel", Namespace: redis.Namespace}, foundSvc)
		if err != nil {
			if apierrors.IsNotFound(err) {
				log.Info("Redis Sentinel ClusterIP Service not found, nothing to delete")
				return ctrl.Result{}, nil
			} else {
				log.Error(err, "Failed to get Redis Sentinel ClusterIP Service",
					"Service.Namespace", foundSvc.Namespace, "Service.Name", foundSvc.Name)
				return ctrl.Result{}, err
			}
		}
		err = r.Delete(ctx, foundSvc)
		if err != nil {
			log.Error(err, "Failed to delete Redis Sentinel ClusterIP Service",
				"Service.Namespace", foundSvc.Namespace, "Service.Name", foundSvc.Name)
			return ctrl.Result{}, err
		}
		log.Info("Deleted Redis Sentinel ClusterIP Service",
			"Service.Namespace", foundSvc.Namespace, "Service.Name", foundSvc.Name)
		return ctrl.Result{}, nil

	}
	return ctrl.Result{}, nil
}

func (r *RedisReconciler) reconcileRedisPodProbeMarker(ctx context.Context, redis *databasev1alpha1.Redis, log logr.Logger) (ctrl.Result, error) {
	foundPpm := &kruisev1alpha1.PodProbeMarker{}
	err := r.Get(ctx, types.NamespacedName{Name: redis.Name + "-podprobemarker", Namespace: redis.Namespace}, foundPpm)
	if err != nil && apierrors.IsNotFound(err) {
		sts, err := resources.PodProbeMarkerForRedis(redis, r.Scheme)
		if err != nil {
			log.Error(err, "Failed to define new PodProbeMarker resource for Redis")
			meta.SetStatusCondition(&redis.Status.Conditions, metav1.Condition{Type: typeAvailableRedis,
				Status: metav1.ConditionFalse, Reason: "Reconciling",
				Message: fmt.Sprintf("Failed to create PodProbeMarker for the custom resource (%s): (%s)", redis.Name, err)})
			if err := r.Status().Update(ctx, redis); err != nil {
				log.Error(err, "Failed to update Redis status")
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		log.Info("Creating a new PodProbeMarker",
			"PodProbeMarker.Namespace", sts.Namespace, "PodProbeMarker.Name", sts.Name)
		if err = r.Create(ctx, sts); err != nil {
			log.Error(err, "Failed to create new PodProbeMarker",
				"PodProbeMarker.Namespace", sts.Namespace, "PodProbeMarker.Name", sts.Name)
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: time.Minute}, nil
	} else if err != nil {
		log.Error(err, "Failed to get PodProbeMarker")
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}
