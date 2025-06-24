package resources

import (
	"bytes"
	"text/template"

	corev1 "k8s.io/api/core/v1"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	databasev1alpha1 "github.com/KateScarlet/redis-operator/api/v1alpha1"
)

const redisConfigTemplate = `# Redis configuration file
bind 0.0.0.0
dir /var/lib/redis
{{if .passFlag}}
requirepass {{ .pass }}
{{end}}
{{if .aofFlag}}
appendonly yes
appendfsync {{ .aofFsync }}
{{end}}
`

func ConfigMapForRedis(redis *databasev1alpha1.Redis, scheme *runtime.Scheme) (*corev1.ConfigMap, error) {
	tmpl, err := template.New("RedisConfig").Parse(redisConfigTemplate)
	if err != nil {
		return nil, err
	}
	pass := redis.Spec.Password
	var passFlag bool
	if pass != "" {
		passFlag = true
	}
	data := map[string]any{
		"pass":     pass,
		"passFlag": passFlag,
		"aofFlag":  redis.Spec.AOF.Enabled,
		"aofFsync": redis.Spec.AOF.Fsync,
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return nil, err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-config",
			Namespace: redis.Namespace,
		},
		Data: map[string]string{
			"redis.conf": buf.String(),
		},
	}

	if err := ctrl.SetControllerReference(redis, cm, scheme); err != nil {
		return nil, err
	}
	return cm, nil
}

const redisSentinelConfigTemplate = `# Redis Sentinel configuration file
sentinel resolve-hostnames yes
sentinel monitor mymaster {{ .redisName }}-0.{{ .redisName }}-headless.{{ .namespace }} 6379 2
sentinel down-after-milliseconds mymaster 5000
sentinel failover-timeout mymaster 10000
sentinel parallel-syncs mymaster 1
{{if .passFlag}}
requirepass {{ .pass }}
sentinel sentinel-pass {{ .pass }}
sentinel auth-pass mymaster {{ .pass }}
{{end}}
`

func ConfigMapForRedisSentinel(redis *databasev1alpha1.Redis, scheme *runtime.Scheme) (*corev1.ConfigMap, error) {
	tmpl, err := template.New("RedisSentinelConfig").Parse(redisSentinelConfigTemplate)
	if err != nil {
		return nil, err
	}
	pass := redis.Spec.Password
	var passFlag bool
	if pass != "" {
		passFlag = true
	}
	data := map[string]any{
		"redisName": redis.Name,
		"pass":      pass,
		"passFlag":  passFlag,
		"namespace": redis.Namespace,
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return nil, err
	}
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      redis.Name + "-sentinel-config",
			Namespace: redis.Namespace,
		},
		Data: map[string]string{
			"sentinel.conf": buf.String(),
		},
	}

	if err := ctrl.SetControllerReference(redis, cm, scheme); err != nil {
		return nil, err
	}
	return cm, nil
}
