package resources

import (
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	databasev1alpha1 "github.com/KateScarlet/redis-operator/api/v1alpha1"
)

func HeadlessServiceForRedis(redis *databasev1alpha1.Redis, scheme *runtime.Scheme) (*corev1.Service, error) {
	servicePorts := []corev1.ServicePort{{
		Name:       "redis-port",
		Port:       6379,
		TargetPort: intstr.FromInt(6379),
	}}

	if redis.Spec.Exporter.Enabled {
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       "exporter-port",
			Port:       9121,
			TargetPort: intstr.FromInt(9121),
		})
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-headless",
			Namespace: redis.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app.kubernetes.io/name": redis.Name},
			Ports:     servicePorts,
			ClusterIP: corev1.ClusterIPNone,
		},
	}
	if err := ctrl.SetControllerReference(redis, svc, scheme); err != nil {
		return nil, err
	}
	return svc, nil
}

func HeadlessServiceForRedisMaster(redis *databasev1alpha1.Redis, scheme *runtime.Scheme) (*corev1.Service, error) {
	servicePorts := []corev1.ServicePort{{
		Name:       "redis-port",
		Port:       6379,
		TargetPort: intstr.FromInt(6379),
	}}

	if redis.Spec.Exporter.Enabled {
		servicePorts = append(servicePorts, corev1.ServicePort{
			Name:       "exporter-port",
			Port:       9121,
			TargetPort: intstr.FromInt(9121),
		})
	}

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-master-headless",
			Namespace: redis.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector:  map[string]string{"app.kubernetes.io/name": redis.Name, "role": "master"},
			Ports:     servicePorts,
			ClusterIP: corev1.ClusterIPNone,
		},
	}
	if err := ctrl.SetControllerReference(redis, svc, scheme); err != nil {
		return nil, err
	}
	return svc, nil
}

func ClusterIPServiceForRedisSentinel(redis *databasev1alpha1.Redis, scheme *runtime.Scheme) (*corev1.Service, error) {
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-sentinel",
			Namespace: redis.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{"app.kubernetes.io/name": redis.Name + "-sentinel"},
			Ports: []corev1.ServicePort{{
				Name:       "sentinel",
				Port:       26379,
				TargetPort: intstr.FromInt(26379),
			}},
		},
	}
	if err := ctrl.SetControllerReference(redis, svc, scheme); err != nil {
		return nil, err
	}
	return svc, nil
}
