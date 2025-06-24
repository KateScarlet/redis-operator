package resources

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	databasev1alpha1 "github.com/KateScarlet/redis-operator/api/v1alpha1"
)

func DeploymentForRedisSentinel(redis *databasev1alpha1.Redis, scheme *runtime.Scheme) (*appsv1.Deployment, error) {
	var (
		replicas int32 = redis.Spec.Sentinel.Replicas
	)
	redisSentinelImage := redis.Spec.Sentinel.Image
	if redisSentinelImage == "" {
		redisSentinelImage = "redis:7.4"
	}

	deploy := &appsv1.Deployment{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      redis.Name + "-sentinel",
			Namespace: redis.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app": redis.Name + "-sentinel",
				},
			},
			Replicas: &replicas,
			Template: corev1.PodTemplateSpec{
				ObjectMeta: ctrl.ObjectMeta{
					Labels: map[string]string{
						"app": redis.Name + "-sentinel",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:    "sentinel",
						Image:   redisSentinelImage,
						Command: []string{"bash", "-c"},
						Args: []string{`
						cp /config/sentinel.conf /etc/sentinel.conf
						exec redis-sentinel /etc/sentinel.conf`},
						Ports: []corev1.ContainerPort{{
							Name:          "sentinel-port",
							ContainerPort: 26379,
						}},
						VolumeMounts: []corev1.VolumeMount{{
							Name:      "redis-sentinel-config",
							MountPath: "/config",
						}},
					}},
					RestartPolicy: "Always",
					Volumes: []corev1.Volume{{
						Name: "redis-sentinel-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: redis.Name + "-sentinel-config",
								},
							},
						},
					}},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(redis, deploy, scheme); err != nil {
		return nil, err
	}
	return deploy, nil
}
