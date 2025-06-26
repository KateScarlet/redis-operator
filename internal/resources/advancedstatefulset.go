package resources

import (
	corev1 "k8s.io/api/core/v1"

	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"

	databasev1alpha1 "github.com/KateScarlet/redis-operator/api/v1alpha1"
	kruisev1beta1 "github.com/openkruise/kruise-api/apps/v1beta1"
)

func AdvancedStatefulSetForRedis(redis *databasev1alpha1.Redis, scheme *runtime.Scheme) (*kruisev1beta1.StatefulSet, error) {
	var (
		replicas   int32              = redis.Spec.Replicas
		Containers []corev1.Container = make([]corev1.Container, 0)
		podArgs    []string
	)
	sentinelArgs := []string{`
cp /conf/redis.conf /etc/redis.conf

STATEFULSET_NAME=$(echo "$POD_NAME" | sed 's/-[0-9]*$//')
SENTINEL_HOST=${STATEFULSET_NAME}-sentinel.${NAMESPACE}

# === Sentinel get-master retry loop ===
MAX_SENTINEL_RETRIES=10
MASTER_IP=""

for i in $(seq 1 $MAX_SENTINEL_RETRIES); do
  MASTER_IP=$(redis-cli --raw -h "${SENTINEL_HOST}" -p 26379 -a "${REDIS_PASSWORD}" --no-auth-warning sentinel get-master-addr-by-name mymaster | head -n 1 || true)
  if [[ "$MASTER_IP" != "" ]]; then
    echo "Discovered master IP from Sentinel: $MASTER_IP"
    break
  fi
  echo "[$i/$MAX_SENTINEL_RETRIES] Waiting for Sentinel to return master..."
  sleep 2
done

# === Decide master or replica ===
MY_IP=$(getent hosts $(hostname -f) | awk '{ print $1 }')
if [[ "${MY_IP}" == "${MASTER_IP}" || "${MASTER_IP}" == "" ]]; then
  echo "Starting as MASTER"
  redis-server /etc/redis.conf
else
  RETRY_COUNT=1
  MASTER_REACHABLE=false
  while [[ $RETRY_COUNT -lt 16 ]]; do
    MASTER_IP=$(redis-cli --raw -h "${SENTINEL_HOST}" -p 26379 -a "${REDIS_PASSWORD}" --no-auth-warning sentinel get-master-addr-by-name mymaster | head -n 1 || true)
    if redis-cli -t 1 --raw -h ${MASTER_IP} -a ${REDIS_PASSWORD} --no-auth-warning ping | grep -q PONG; then
      MASTER_REACHABLE=true
      break
    fi
    echo "[$RETRY_COUNT/15] Ping to MASTER $MASTER_IP failed, retrying..."
    sleep 1
    RETRY_COUNT=$((RETRY_COUNT + 1))
  done
  if [[ "$MASTER_REACHABLE" == "false" ]]; then
    echo "Cannot reach MASTER at $MASTER_IP after retries. Starting as fallback MASTER"
    exec redis-server /etc/redis.conf
  fi
  echo "Starting as SLAVE of ${MASTER_IP}"
  echo "replicaof ${MASTER_IP} 6379" >> /etc/redis.conf 
  echo "masterauth ${REDIS_PASSWORD}" >> /etc/redis.conf
  exec redis-server /etc/redis.conf
fi
`}
	noSentinelArgs := []string{`
cp /conf/redis.conf /etc/redis.conf
STATEFULSET_NAME=$(echo "${POD_NAME}" | sed 's/-[0-9]*$//')
ORDINAL=$(hostname | awk -F'-' '{print $NF}')
if [ "$ORDINAL" = "0" ]; then
  exec redis-server /etc/redis.conf
else
  echo "replicaof ${STATEFULSET_NAME}-0.${STATEFULSET_NAME}-headless.${NAMESPACE} 6379" >> /etc/redis.conf 
  echo "masterauth ${REDIS_PASSWORD}" >> /etc/redis.conf 
  exec redis-server /etc/redis.conf 
fi
`}
	if redis.Spec.Sentinel.Enabled {
		podArgs = sentinelArgs
	} else {
		podArgs = noSentinelArgs
	}
	redisContainer := corev1.Container{
		Image:           redis.Spec.Image,
		Name:            "redis",
		ImagePullPolicy: corev1.PullIfNotPresent,
		Ports: []corev1.ContainerPort{{
			ContainerPort: 6379,
			Name:          "redis-port",
		}},
		Command: []string{"/bin/bash", "-c"},
		Args:    podArgs,
		LivenessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(6379),
				},
			},
			InitialDelaySeconds: 30,
			TimeoutSeconds:      2,
			PeriodSeconds:       10,
			SuccessThreshold:    1,
			FailureThreshold:    3,
		},
		ReadinessProbe: &corev1.Probe{
			ProbeHandler: corev1.ProbeHandler{
				TCPSocket: &corev1.TCPSocketAction{
					Port: intstr.FromInt(6379),
				},
			},
			InitialDelaySeconds: 5,
			TimeoutSeconds:      2,
			PeriodSeconds:       5,
			SuccessThreshold:    1,
			FailureThreshold:    3,
		},
		Env: []corev1.EnvVar{{
			Name: "POD_NAME",
			ValueFrom: &corev1.EnvVarSource{
				FieldRef: &corev1.ObjectFieldSelector{
					FieldPath: "metadata.name",
				},
			},
		}, {
			Name:  "REDIS_PASSWORD",
			Value: redis.Spec.Password,
		}, {
			Name:  "NAMESPACE",
			Value: redis.Namespace,
		},
		},
		Resources: corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    *redis.Spec.Resources.Requests.Cpu(),
				corev1.ResourceMemory: *redis.Spec.Resources.Requests.Memory(),
			},
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    *redis.Spec.Resources.Limits.Cpu(),
				corev1.ResourceMemory: *redis.Spec.Resources.Limits.Memory(),
			},
		},
		VolumeMounts: []corev1.VolumeMount{{
			Name:      "data",
			MountPath: "/var/lib/redis",
		}, {
			Name:      redis.Name + "-config",
			MountPath: "/conf",
		}},
	}
	Containers = append(Containers, redisContainer)

	if redis.Spec.Exporter.Enabled {
		exporterImage := redis.Spec.Exporter.Image
		if exporterImage == "" {
			exporterImage = "oliver006/redis_exporter:latest"
		}
		exporterContainer := corev1.Container{
			Image:           exporterImage,
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
					corev1.ResourceCPU:    *redis.Spec.Sentinel.Resources.Requests.Cpu(),
					corev1.ResourceMemory: *redis.Spec.Sentinel.Resources.Requests.Memory(),
				},
				Limits: corev1.ResourceList{
					corev1.ResourceCPU:    *redis.Spec.Sentinel.Resources.Limits.Cpu(),
					corev1.ResourceMemory: *redis.Spec.Sentinel.Resources.Limits.Memory(),
				},
			},
		}
		Containers = append(Containers, exporterContainer)
	}

	var stsVolumeClaimTemplates []corev1.PersistentVolumeClaim
	if true {
		volumeSize := redis.Spec.Volume.Size
		if volumeSize == "" {
			volumeSize = "10Gi"
		}
		stsVolumeClaimTemplates = []corev1.PersistentVolumeClaim{{
			ObjectMeta: metav1.ObjectMeta{
				Name: "data",
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: resource.MustParse(volumeSize),
					},
				},
				StorageClassName: redis.Spec.Volume.StorageClass,
			},
		}}
	}

	sts := &kruisev1beta1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name,
			Namespace: redis.Namespace,
		},
		Spec: kruisev1beta1.StatefulSetSpec{
			Replicas:    &replicas,
			ServiceName: redis.Name + "-headless",
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/name": redis.Name},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/name": redis.Name},
				},
				Spec: corev1.PodSpec{
					Containers: Containers,
					TopologySpreadConstraints: []corev1.TopologySpreadConstraint{{
						MaxSkew:           1,
						TopologyKey:       "kubernetes.io/hostname",
						WhenUnsatisfiable: corev1.ScheduleAnyway,
						LabelSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app.kubernetes.io/name": redis.Name,
							},
						},
					}},
					Volumes: []corev1.Volume{{
						Name: redis.Name + "-config",
						VolumeSource: corev1.VolumeSource{
							ConfigMap: &corev1.ConfigMapVolumeSource{
								LocalObjectReference: corev1.LocalObjectReference{
									Name: redis.Name + "-config",
								},
								Items: []corev1.KeyToPath{{
									Key:  "redis.conf",
									Path: "redis.conf",
								}},
							},
						},
					}},
				},
			},
			VolumeClaimTemplates: stsVolumeClaimTemplates,
		},
	}

	if err := ctrl.SetControllerReference(redis, sts, scheme); err != nil {
		return nil, err
	}
	return sts, nil
}
