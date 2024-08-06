package main

import (
    "encoding/json"
    "io/ioutil"
    "net/http"
    "os"

    admissionv1 "k8s.io/api/admission/v1"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/runtime"
    "k8s.io/apimachinery/pkg/runtime/serializer"
    "k8s.io/klog/v2"
)

var (
    scheme = runtime.NewScheme()
    codecs = serializer.NewCodecFactory(scheme)
)

func main() {
    http.HandleFunc("/mutate", mutatePods)
    server := &http.Server{
        Addr: ":8443",
    }
    klog.Info("Starting webhook server")
    if err := server.ListenAndServeTLS("/etc/webhook/certs/tls.crt", "/etc/webhook/certs/tls.key"); err != nil {
        klog.Fatalf("Failed to listen and serve webhook server: %v", err)
    }
}

func mutatePods(w http.ResponseWriter, r *http.Request) {
    var admissionReview admissionv1.AdmissionReview
    body, err := ioutil.ReadAll(r.Body)
    if err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }
    if _, _, err := codecs.UniversalDeserializer().Decode(body, nil, &admissionReview); err != nil {
        http.Error(w, err.Error(), http.StatusBadRequest)
        return
    }

    pod := corev1.Pod{}
    if err := json.Unmarshal(admissionReview.Request.Object.Raw, &pod); err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    sidecar := corev1.Container{
        Name:  "konnectivity-agent",
        Image: "konnectivity-agent:latest",
        Args: []string{
            "--agent-identifiers=ipv4=$(POD_IP)",
        },
        Env: []corev1.EnvVar{
            {
                Name: "POD_IP",
                ValueFrom: &corev1.EnvVarSource{
                    FieldRef: &corev1.ObjectFieldSelector{
                        FieldPath: "status.podIP",
                    },
                },
            },
        },
    }

    pod.Spec.Containers = append(pod.Spec.Containers, sidecar)
    marshaledPod, err := json.Marshal(pod)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    admissionResponse := admissionv1.AdmissionResponse{
        Allowed: true,
        Patch:   marshaledPod,
        PatchType: func() *admissionv1.PatchType {
            pt := admissionv1.PatchTypeJSONPatch
            return &pt
        }(),
    }

    admissionReview.Response = &admissionResponse
    response, err := json.Marshal(admissionReview)
    if err != nil {
        http.Error(w, err.Error(), http.StatusInternalServerError)
        return
    }

    w.Write(response)
}
