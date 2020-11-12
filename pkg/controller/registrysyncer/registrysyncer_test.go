package registrysyncer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/go-cmp/cmp"
	"github.com/sirupsen/logrus"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/sets"
	"k8s.io/client-go/kubernetes/scheme"
	ctrlruntimeclient "sigs.k8s.io/controller-runtime/pkg/client"
	fakeclient "sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	imagev1 "github.com/openshift/api/image/v1"

	"github.com/openshift/ci-tools/pkg/testhelper"
)

func TestPublicDomainForImage(t *testing.T) {
	testCases := []struct {
		name               string
		clusterName        string
		potentiallyPrivate string
		expected           string
		expectedError      error
	}{
		{
			name:               "app.ci with svc dns",
			clusterName:        "app.ci",
			potentiallyPrivate: "image-registry.openshift-image-registry.svc:5000/ci/applyconfig@sha256:bf08a76268b29f056cfab7a105c8473b359d1154fbbe3091fe6052ad6d0427cd",
			expected:           "registry.ci.openshift.org/ci/applyconfig@sha256:bf08a76268b29f056cfab7a105c8473b359d1154fbbe3091fe6052ad6d0427cd",
		},
		{
			name:               "api.ci with svc dns",
			clusterName:        "api.ci",
			potentiallyPrivate: "docker-registry.default.svc:5000/ci/applyconfig@sha256:bf08a76268b29f056cfab7a105c8473b359d1154fbbe3091fe6052ad6d0427cd",
			expected:           "registry.svc.ci.openshift.org/ci/applyconfig@sha256:bf08a76268b29f056cfab7a105c8473b359d1154fbbe3091fe6052ad6d0427cd",
		},
		{
			name:               "api.ci with public domain",
			clusterName:        "api.ci",
			potentiallyPrivate: "gcr.io/k8s-prow/tide@sha256:5245b7747c44d560aab27bc07dbaaf50bbb55f71d0973f85b09c79b8d8b93c97",
			expected:           "gcr.io/k8s-prow/tide@sha256:5245b7747c44d560aab27bc07dbaaf50bbb55f71d0973f85b09c79b8d8b93c97",
		},
		{
			name:               "app.ci with public domain",
			clusterName:        "app.ci",
			potentiallyPrivate: "gcr.io/k8s-prow/tide@sha256:5245b7747c44d560aab27bc07dbaaf50bbb55f71d0973f85b09c79b8d8b93c97",
			expected:           "gcr.io/k8s-prow/tide@sha256:5245b7747c44d560aab27bc07dbaaf50bbb55f71d0973f85b09c79b8d8b93c97",
		},
		{
			name:               "unknown context",
			clusterName:        "unknown",
			potentiallyPrivate: "gcr.io/k8s-prow/tide@sha256:5245b7747c44d560aab27bc07dbaaf50bbb55f71d0973f85b09c79b8d8b93c97",
			expected:           "",
			expectedError:      fmt.Errorf("failed to get the domain for cluster unknown"),
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, actualError := publicDomainForImage(tc.clusterName, tc.potentiallyPrivate)
			if diff := cmp.Diff(tc.expected, actual); diff != "" {
				t.Errorf("actual does not match expected, diff: %s", diff)
			}
			if diff := cmp.Diff(tc.expectedError, actualError, testhelper.EquateErrorMessage); diff != "" {
				t.Errorf("actualError does not match expectedError, diff: %s", diff)
			}
		})
	}
}

func TestFindNewest(t *testing.T) {
	now := time.Now()
	testCases := []struct {
		name     string
		isTags   map[string]*imagev1.ImageStreamTag
		expected string
	}{
		{
			name: "nil isTags",
		},
		{
			name:   "empty isTags",
			isTags: map[string]*imagev1.ImageStreamTag{},
		},
		{
			name: "basic case: 2 clusters",
			isTags: map[string]*imagev1.ImageStreamTag{
				"cluster1": {
					Image: imagev1.Image{
						ObjectMeta: metav1.ObjectMeta{
							CreationTimestamp: metav1.NewTime(now),
						},
					},
				},
				"cluster2": {
					Image: imagev1.Image{
						ObjectMeta: metav1.ObjectMeta{
							CreationTimestamp: metav1.NewTime(now.Add(-1 * time.Minute)),
						},
					},
				},
			},
			expected: "cluster1",
		},
		{
			name: "3 of them",
			isTags: map[string]*imagev1.ImageStreamTag{
				"cluster1": {
					Image: imagev1.Image{
						ObjectMeta: metav1.ObjectMeta{
							CreationTimestamp: metav1.NewTime(now),
						},
					},
				},
				"cluster2": {
					Image: imagev1.Image{
						ObjectMeta: metav1.ObjectMeta{
							CreationTimestamp: metav1.NewTime(now.Add(1 * time.Minute)),
						},
					},
				},
				"cluster3": {
					Image: imagev1.Image{
						ObjectMeta: metav1.ObjectMeta{
							CreationTimestamp: metav1.NewTime(now.Add(-1 * time.Minute)),
						},
					},
				},
			},
			expected: "cluster2",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual := findNewest(tc.isTags)
			if diff := cmp.Diff(tc.expected, actual); diff != "" {
				t.Errorf("actual does not match expected, diff: %s", diff)
			}
		})
	}
}

const (
	apiCI = "api.ci"
	appCI = "app.ci"
)

func init() {
	if err := imagev1.Install(scheme.Scheme); err != nil {
		panic(fmt.Sprintf("failed to register imagev1 scheme: %v", err))
	}
}

func TestReconcile(t *testing.T) {
	pullSecretGetter := func() []byte {
		return []byte("some-secret")
	}

	applyconfigISTag := &imagev1.ImageStreamTag{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ci",
			Name:      "applyconfig:latest",
		},
		Image: imagev1.Image{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "sha256:4ff455dca5145a078c263ebf716eb1ccd1fe6fd41c9f9de6f27a9af9bbb0349d",
				CreationTimestamp: metav1.Now(),
			},
			DockerImageReference: "docker-registry.default.svc:5000/ci/applyconfig@sha256:4ff455dca5145a078c263ebf716eb1ccd1fe6fd41c9f9de6f27a9af9bbb0349d",
		},
	}

	applyconfigISTagNewer := &imagev1.ImageStreamTag{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ci",
			Name:      "applyconfig:latest",
		},
		Image: imagev1.Image{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "sha256:new",
				CreationTimestamp: metav1.NewTime(metav1.Now().Add(3 * time.Minute)),
			},
			DockerImageReference: "image-registry.openshift-image-registry.svc:5000/ci/applyconfig@sha256:new",
		},
	}

	applyconfigISTagNewerSameName := &imagev1.ImageStreamTag{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ci",
			Name:      "applyconfig:latest",
		},
		Image: imagev1.Image{
			ObjectMeta: metav1.ObjectMeta{
				Name:              "sha256:4ff455dca5145a078c263ebf716eb1ccd1fe6fd41c9f9de6f27a9af9bbb0349d",
				CreationTimestamp: metav1.NewTime(metav1.Now().Add(3 * time.Minute)),
			},
			DockerImageReference: "image-registry.openshift-image-registry.svc:5000/ci/applyconfig@sha256:4ff455dca5145a078c263ebf716eb1ccd1fe6fd41c9f9de6f27a9af9bbb0349d",
		},
	}

	applyconfigIS := &imagev1.ImageStream{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: "ci",
			Name:      "applyconfig",
			Annotations: map[string]string{
				"release.openshift.io-something": "copied",
				"something":                      "not-copied",
			},
		},
	}

	ctx := context.Background()

	for _, tc := range []struct {
		name                 string
		request              types.NamespacedName
		apiCIClient          ctrlruntimeclient.Client
		appCIClient          ctrlruntimeclient.Client
		expected             error
		expectedAPICIObjects []runtime.Object
		expectedAPPCIObjects []runtime.Object
		verify               func(apiCIClient ctrlruntimeclient.Client, appCIClient ctrlruntimeclient.Client) error
	}{
		{
			name: "abnormal case: the underlying imagestream is gone",
			request: types.NamespacedName{
				Name:      "applyconfig:latest",
				Namespace: "ci",
			},
			apiCIClient: fakeclient.NewFakeClient(applyconfigISTag.DeepCopy()),
			appCIClient: fakeclient.NewFakeClient(),
			expected:    fmt.Errorf("failed to get imageStream %s from registry cluster: %w", "ci/applyconfig", fmt.Errorf("imagestreams.image.openshift.io \"applyconfig\" not found")),
		},
		{
			name: "a new tag",
			request: types.NamespacedName{
				Name:      "applyconfig:latest",
				Namespace: "ci",
			},
			apiCIClient: fakeclient.NewFakeClient(applyconfigISTag.DeepCopy(), applyconfigIS.DeepCopy()),
			appCIClient: bcc(fakeclient.NewFakeClient()),

			verify: func(apiCIClient ctrlruntimeclient.Client, appCIClient ctrlruntimeclient.Client) error {
				actualImageStreamImport := &imagev1.ImageStreamImport{}
				if err := appCIClient.Get(ctx, types.NamespacedName{Name: "applyconfig", Namespace: "ci"}, actualImageStreamImport); err != nil {
					return err
				}
				expectedImageStreamImport := &imagev1.ImageStreamImport{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ImageStreamImport",
						APIVersion: "image.openshift.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "ci",
						Name:            "applyconfig",
						ResourceVersion: "1",
					},
					Spec: imagev1.ImageStreamImportSpec{
						Import: true,
						Images: []imagev1.ImageImportSpec{{
							From: corev1.ObjectReference{
								Kind: "DockerImage",
								Name: "registry.svc.ci.openshift.org/ci/applyconfig@sha256:4ff455dca5145a078c263ebf716eb1ccd1fe6fd41c9f9de6f27a9af9bbb0349d",
							},
							To: &corev1.LocalObjectReference{Name: "latest"},
							ReferencePolicy: imagev1.TagReferencePolicy{
								Type: imagev1.LocalTagReferencePolicy,
							},
						}},
					},
					Status: imagev1.ImageStreamImportStatus{
						Images: []imagev1.ImageImportStatus{
							{
								Image: &imagev1.Image{},
							},
						},
					},
				}
				if diff := cmp.Diff(expectedImageStreamImport, actualImageStreamImport); diff != "" {
					return fmt.Errorf("actual does not match expected, diff: %s", diff)
				}

				actualImageStream := &imagev1.ImageStream{}
				if err := appCIClient.Get(ctx, types.NamespacedName{Name: "applyconfig", Namespace: "ci"}, actualImageStream); err != nil {
					return err
				}
				expectedImageStream := &imagev1.ImageStream{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ImageStream",
						APIVersion: "image.openshift.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "ci",
						Name:            "applyconfig",
						ResourceVersion: "1",
						Annotations: map[string]string{
							"release.openshift.io-something": "copied",
						},
					},
				}
				if diff := cmp.Diff(expectedImageStream, actualImageStream); diff != "" {
					return fmt.Errorf("actual does not match expected, diff: %s", diff)
				}

				actualNamespace := &corev1.Namespace{}
				if err := appCIClient.Get(ctx, types.NamespacedName{Name: "ci"}, actualNamespace); err != nil {
					return err
				}
				expectedNamespace := &corev1.Namespace{
					TypeMeta: metav1.TypeMeta{
						Kind:       "Namespace",
						APIVersion: "v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Name:            "ci",
						ResourceVersion: "1",
						Annotations: map[string]string{
							"dptp.openshift.io/requester": "registry_syncer",
						},
					},
				}
				if diff := cmp.Diff(expectedNamespace, actualNamespace); diff != "" {
					return fmt.Errorf("actual does not match expected, diff: %s", diff)
				}
				return nil
			},
		},
		{
			name: "app.ci is newer",
			request: types.NamespacedName{
				Name:      "applyconfig:latest",
				Namespace: "ci",
			},
			apiCIClient: bcc(fakeclient.NewFakeClient(applyconfigISTag.DeepCopy())),
			appCIClient: fakeclient.NewFakeClient(applyconfigISTagNewer.DeepCopy(), applyconfigIS.DeepCopy()),

			verify: func(apiCIClient ctrlruntimeclient.Client, appCIClient ctrlruntimeclient.Client) error {
				actualImageStreamImport := &imagev1.ImageStreamImport{}
				if err := apiCIClient.Get(ctx, types.NamespacedName{Name: "applyconfig", Namespace: "ci"}, actualImageStreamImport); err != nil {
					return err
				}
				expectedImageStreamImport := &imagev1.ImageStreamImport{
					TypeMeta: metav1.TypeMeta{
						Kind:       "ImageStreamImport",
						APIVersion: "image.openshift.io/v1",
					},
					ObjectMeta: metav1.ObjectMeta{
						Namespace:       "ci",
						Name:            "applyconfig",
						ResourceVersion: "1",
					},
					Spec: imagev1.ImageStreamImportSpec{
						Import: true,
						Images: []imagev1.ImageImportSpec{{
							From: corev1.ObjectReference{
								Kind: "DockerImage",
								Name: "registry.ci.openshift.org/ci/applyconfig@sha256:new",
							},
							To: &corev1.LocalObjectReference{Name: "latest"},
							ReferencePolicy: imagev1.TagReferencePolicy{
								Type: imagev1.LocalTagReferencePolicy,
							},
						}},
					},
					Status: imagev1.ImageStreamImportStatus{
						Images: []imagev1.ImageImportStatus{
							{
								Image: &imagev1.Image{},
							},
						},
					},
				}
				if diff := cmp.Diff(expectedImageStreamImport, actualImageStreamImport); diff != "" {
					return fmt.Errorf("actual does not match expected, diff: %s", diff)
				}
				return nil
			},
		},
		{
			name: "app.ci is newer but refers to the same image",
			request: types.NamespacedName{
				Name:      "applyconfig:latest",
				Namespace: "ci",
			},
			apiCIClient: fakeclient.NewFakeClient(applyconfigISTag.DeepCopy()),
			appCIClient: fakeclient.NewFakeClient(applyconfigISTagNewerSameName.DeepCopy(), applyconfigIS.DeepCopy()),

			verify: func(apiCIClient ctrlruntimeclient.Client, appCIClient ctrlruntimeclient.Client) error {
				for clusterName, client := range map[string]ctrlruntimeclient.Client{apiCI: apiCIClient, appCI: appCIClient} {
					// We could check if client.List()==0 (optionally with ctrlruntimeclient.InNamespace("ci"))
					// if imagev1.ImageStreamImportList is available
					actualImageStreamImport := &imagev1.ImageStreamImport{}
					err := client.Get(ctx, types.NamespacedName{Name: "applyconfig", Namespace: "ci"}, actualImageStreamImport)
					if !errors.IsNotFound(err) {
						return fmt.Errorf("unexpected import on %s", clusterName)
					}
				}
				return nil
			},
		},
		{
			name: "import check failed",
			request: types.NamespacedName{
				Name:      "applyconfig:latest",
				Namespace: "ci",
			},
			apiCIClient: fakeclient.NewFakeClient(applyconfigISTag.DeepCopy(), applyconfigIS.DeepCopy()),
			appCIClient: bcc(fakeclient.NewFakeClient(), func(c *imageImportStatusSettingClient) { c.failure = true }),
			expected:    fmt.Errorf("imageStreamImport did not succeed: reason: , message: failing as requested"),
		},
	} {
		if tc.name != "a new tag" {
			continue
		}
		t.Run(tc.name, func(t *testing.T) {
			r := &reconciler{
				log: logrus.NewEntry(logrus.New()),
				registryClients: map[string]ctrlruntimeclient.Client{
					apiCI: tc.apiCIClient,
					appCI: tc.appCIClient,
				},
				pullSecretGetter: pullSecretGetter,
			}

			request := reconcile.Request{NamespacedName: tc.request}
			actual := r.reconcile(context.Background(), request, r.log)

			if diff := cmp.Diff(tc.expected, actual, testhelper.EquateErrorMessage); diff != "" {
				t.Errorf("actualError does not match expectedError, diff: %s", diff)
			}
			if actual == nil && tc.verify != nil {
				if err := tc.verify(tc.apiCIClient, tc.appCIClient); err != nil {
					t.Errorf("unexpcected error: %v", err)
				}
			}
		})
	}
}

func bcc(upstream ctrlruntimeclient.Client, opts ...func(*imageImportStatusSettingClient)) ctrlruntimeclient.Client {
	c := &imageImportStatusSettingClient{
		Client: upstream,
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

type imageImportStatusSettingClient struct {
	ctrlruntimeclient.Client
	failure bool
}

func (client *imageImportStatusSettingClient) Create(ctx context.Context, obj ctrlruntimeclient.Object, opts ...ctrlruntimeclient.CreateOption) error {
	if asserted, match := obj.(*imagev1.ImageStreamImport); match {
		asserted.Status.Images = []imagev1.ImageImportStatus{{}}
		if client.failure {
			asserted.Status.Images[0].Status.Message = "failing as requested"
		} else {
			asserted.Status.Images[0].Image = &imagev1.Image{}
		}
	}
	return client.Client.Create(ctx, obj, opts...)
}

func TestTestInputImageStreamTagFilterFactory(t *testing.T) {
	testCases := []struct {
		name                  string
		l                     *logrus.Entry
		imageStreamTags       sets.String
		imageStreams          sets.String
		imageStreamNamespaces sets.String
		nn                    types.NamespacedName
		expected              bool
	}{
		{
			name: "default",
			nn:   types.NamespacedName{Namespace: "some-namespace", Name: "some-name:some-tag"},
		},
		{
			name:            "imageStreamTags",
			nn:              types.NamespacedName{Namespace: "some-namespace", Name: "some-name:some-tag"},
			imageStreamTags: sets.NewString("some-namespace/some-name:some-tag"),
			expected:        true,
		},
		{
			name:         "imageStreams",
			nn:           types.NamespacedName{Namespace: "some-namespace", Name: "some-name:some-tag"},
			imageStreams: sets.NewString("some-namespace/some-name"),
			expected:     true,
		},
		{
			name:                  "imageStreamNamespaces",
			nn:                    types.NamespacedName{Namespace: "some-namespace", Name: "some-name:some-tag"},
			imageStreamNamespaces: sets.NewString("some-namespace"),
			expected:              true,
		},
		{
			name: "not valid isTag name",
			nn:   types.NamespacedName{Namespace: "some-namespace", Name: "not-valid-name"},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			tc.l = logrus.WithField("tc.name", tc.name)
			objectFilter := testInputImageStreamTagFilterFactory(tc.l, tc.imageStreamTags, tc.imageStreams, tc.imageStreamNamespaces)
			if diff := cmp.Diff(tc.expected, objectFilter(tc.nn)); diff != "" {
				t.Errorf("actual does not match expected, diff: %s", diff)
			}
		})
	}
}
