// Copyright (c) 2018 Intel Corporation
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

package webhook

import (
	"bytes"
	"encoding/json"

	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/intel/multus-cni/types"
	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"

	"k8s.io/api/admission/v1beta1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

var _ = Describe("Webhook", func() {
	Describe("Mutating", func() {
		var oldGetNetworkAttachmentDefinition = getNetworkAttachmentDefinition

		BeforeEach(func() {
			oldGetNetworkAttachmentDefinition = getNetworkAttachmentDefinition
		})

		AfterEach(func() {
			getNetworkAttachmentDefinition = oldGetNetworkAttachmentDefinition
		})

		DescribeTable("Network Attachment Definition validation",
			func(networkDefinition types.NetworkAttachmentDefinition, pod corev1.Pod, expected []jsonPatchOperation, shouldFail bool) {
				getNetworkAttachmentDefinition = func(namespace, name string) (*types.NetworkAttachmentDefinition, error) {
					return &networkDefinition, nil
				}

				jsonPod, err := json.Marshal(&pod)
				Expect(err).NotTo(HaveOccurred())

				ar := v1beta1.AdmissionReview{
					Request: &v1beta1.AdmissionRequest{
						Object: runtime.RawExtension{
							Raw: jsonPod,
						}},
					TypeMeta: metav1.TypeMeta{
						Kind: "AdmissionReview",
					}}

				b, err := json.Marshal(&ar)
				Expect(err).NotTo(HaveOccurred())

				req := httptest.NewRequest("POST", fmt.Sprintf("https://fakewebhook/validate"), bytes.NewReader(b))
				req.Header.Set("Content-Type", "application/json")
				w := httptest.NewRecorder()
				MutateHandler(w, req)
				resp := w.Result()

				Expect(resp.StatusCode).To(Equal(http.StatusOK))

				d := json.NewDecoder(resp.Body)
				result := v1beta1.AdmissionReview{}
				d.Decode(&result)
				var patchToApply []jsonPatchOperation
				json.Unmarshal(result.Response.Patch, &patchToApply)

				Expect(patchToApply).To(Equal(expected))
			},
			Entry(
				"empty requests",
				types.NetworkAttachmentDefinition{
					Metadata: metav1.ObjectMeta{
						Annotations: map[string]string{
							networkResourceNameKey: "openshift.io/intel_sriov",
						},
					},
				},
				corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"k8s.v1.cni.cncf.io/networks": "[{\"interface\":\"net1\",\"name\":\"sriov\",\"namespace\":\"kubevirt-test-default\"}]",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{corev1.Container{}},
					},
				},
				[]jsonPatchOperation{
					jsonPatchOperation{"add", "/spec/containers/0/resources/requests", map[string]interface{}{"openshift.io/intel_sriov": "1"}},
					jsonPatchOperation{"add", "/spec/containers/0/resources/limits", map[string]interface{}{"openshift.io/intel_sriov": "1"}},
				}, false,
			),
			Entry(
				"pod already having requests",
				types.NetworkAttachmentDefinition{
					Metadata: metav1.ObjectMeta{
						Annotations: map[string]string{
							networkResourceNameKey: "openshift.io/intel_sriov",
						},
					},
				},
				corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Annotations: map[string]string{
							"k8s.v1.cni.cncf.io/networks": "[{\"interface\":\"net1\",\"name\":\"sriov\",\"namespace\":\"kubevirt-test-default\"}]",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{corev1.Container{
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									"cpu":    *resource.NewQuantity(100, resource.DecimalSI),
									"memory": *resource.NewQuantity(12345, resource.DecimalSI),
								},
								Requests: corev1.ResourceList{
									"cpu":    *resource.NewQuantity(100, resource.DecimalSI),
									"memory": *resource.NewQuantity(12345, resource.DecimalSI),
								},
							},
						}},
					},
				},
				[]jsonPatchOperation{
					jsonPatchOperation{"add", "/spec/containers/0/resources/requests",
						map[string]interface{}{
							"openshift.io/intel_sriov": "1",
							"cpu":                      "100",
							"memory":                   "12345"}},
					jsonPatchOperation{"add", "/spec/containers/0/resources/limits",
						map[string]interface{}{
							"openshift.io/intel_sriov": "1",
							"cpu":                      "100",
							"memory":                   "12345"}},
				}, false,
			),
		)
	},
	)
},
)
