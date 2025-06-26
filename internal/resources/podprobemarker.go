package resources

import (
	"bytes"
	"text/template"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"

	databasev1alpha1 "github.com/KateScarlet/redis-operator/api/v1alpha1"
	kruisev1alpha1 "github.com/openkruise/kruise-api/apps/v1alpha1"
)

var probeCommandTemplate = `ROLE=$(redis-cli -a "{{ .pass }}" ROLE 2>/dev/null | awk 'NR==1{print $1}' | tr -d '\r')
if [ "$ROLE" = "master" ]; then
    exit 0
else
    exit 1
fi
`

func PodProbeMarkerForRedis(redis *databasev1alpha1.Redis, scheme *runtime.Scheme) (*kruisev1alpha1.PodProbeMarker, error) {
	tmpl, err := template.New("probeCommand").Parse(probeCommandTemplate)
	if err != nil {
		return nil, err
	}
	data := map[string]any{
		"pass": redis.Spec.Password,
	}
	var buf bytes.Buffer
	err = tmpl.Execute(&buf, data)
	if err != nil {
		return nil, err
	}
	ppm := &kruisev1alpha1.PodProbeMarker{
		ObjectMeta: ctrl.ObjectMeta{
			Name:      redis.Name + "-podprobemarker",
			Namespace: redis.Namespace,
		},
		Spec: kruisev1alpha1.PodProbeMarkerSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app.kubernetes.io/name": redis.Name,
				},
			},
			Probes: []kruisev1alpha1.PodContainerProbe{{
				Name:          "RedisRole",
				ContainerName: "redis",
				Probe: kruisev1alpha1.ContainerProbeSpec{
					Probe: corev1.Probe{
						ProbeHandler: corev1.ProbeHandler{
							Exec: &corev1.ExecAction{
								Command: []string{"/bin/bash", "-c", buf.String()},
							},
						},
						InitialDelaySeconds: 40,
						PeriodSeconds:       2,
						FailureThreshold:    3,
					},
				},
				MarkerPolicy: []kruisev1alpha1.ProbeMarkerPolicy{{
					State: "Succeeded",
					Labels: map[string]string{
						"role": "master",
					},
				}, {
					State: "Failed",
					Labels: map[string]string{
						"role": "slave",
					},
				}},
			}},
		},
	}
	return ppm, nil
}
