package strategy

import (
	"fmt"
	"path/filepath"

	"k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/serializer"

	buildv1 "github.com/openshift/api/build/v1"
	buildapi "github.com/openshift/origin/pkg/build/apis/build"
	buildinstall "github.com/openshift/origin/pkg/build/apis/build/install"
	buildutil "github.com/openshift/origin/pkg/build/util"
)

var (
	buildEncodingScheme       = runtime.NewScheme()
	buildEncodingCodecFactory = serializer.NewCodecFactory(buildEncodingScheme)
	buildJSONCodec            runtime.Encoder
)

func init() {
	// TODO only use external versions, so we only add external types
	buildinstall.Install(buildEncodingScheme)
	buildJSONCodec = buildEncodingCodecFactory.LegacyCodec(buildv1.SchemeGroupVersion)
}

// DockerBuildStrategy creates a Docker build using a Docker builder image.
type DockerBuildStrategy struct {
	Image string
}

// CreateBuildPod creates the pod to be used for the Docker build
// TODO: Make the Pod definition configurable
func (bs *DockerBuildStrategy) CreateBuildPod(build *buildapi.Build) (*v1.Pod, error) {
	data, err := runtime.Encode(buildJSONCodec, build)
	if err != nil {
		return nil, fmt.Errorf("failed to encode the build: %v", err)
	}

	privileged := true
	strategy := build.Spec.Strategy.DockerStrategy

	containerEnv := []v1.EnvVar{
		{Name: "BUILD", Value: string(data)},
	}

	addSourceEnvVars(build.Spec.Source, &containerEnv)

	if len(strategy.Env) > 0 {
		buildutil.MergeTrustedEnvWithoutDuplicates(buildutil.CopyApiEnvVarToV1EnvVar(strategy.Env), &containerEnv, true)
	}

	serviceAccount := build.Spec.ServiceAccount
	if len(serviceAccount) == 0 {
		serviceAccount = buildutil.BuilderServiceAccountName
	}

	buildContextDir := buildutil.InputContentPath
	if build.Spec.Source.ContextDir != "" {
		buildContextDir = filepath.Join(buildContextDir, build.Spec.Source.ContextDir)
	}
	var buildTagArgs []string
	if build.Spec.Output.To != nil {
		switch build.Spec.Output.To.Kind {
		case "DockerImage":
			buildTagArgs = []string{"-t", "docker://" + build.Spec.Output.To.Name}
		default:
			return nil, fmt.Errorf("unhandled output kind %q", build.Spec.Output.To.Kind)
		}
	}
	var buildLabelArgs []string
	for _, imageLabel := range build.Spec.Output.ImageLabels {
		buildLabelArgs = append(buildLabelArgs, "--label", fmt.Sprintf("%s=%s", imageLabel.Name, imageLabel.Value))
	}
	buildCommand := append(append(append([]string{"buildah", "build-using-dockerfile", "--tls-verify=false"}, buildTagArgs...), buildLabelArgs...), buildContextDir)
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      buildapi.GetBuildPodName(build),
			Namespace: build.Namespace,
			Labels:    getPodLabels(build),
		},
		Spec: v1.PodSpec{
			ServiceAccountName: serviceAccount,
			Containers: []v1.Container{
				{
					Name:    DockerBuild,
					Image:   "quay.io/nalind/buildah-chroot-wip:chroot-wip",
					Command: buildCommand,
					Env:     copyEnvVarSlice(containerEnv),
					// TODO: run unprivileged https://github.com/openshift/origin/issues/662
					SecurityContext: &v1.SecurityContext{
						Privileged: &privileged,
					},
					TerminationMessagePolicy: v1.TerminationMessageFallbackToLogsOnError,
					VolumeMounts: []v1.VolumeMount{
						{
							Name:      "buildworkdir",
							MountPath: buildutil.BuildWorkDirMount,
						},
					},
					ImagePullPolicy: v1.PullIfNotPresent,
					Resources:       buildutil.CopyApiResourcesToV1Resources(&build.Spec.Resources),
				},
			},
			Volumes: []v1.Volume{
				{
					Name: "buildworkdir",
					VolumeSource: v1.VolumeSource{
						EmptyDir: &v1.EmptyDirVolumeSource{},
					},
				},
			},
			RestartPolicy: v1.RestartPolicyNever,
			NodeSelector:  build.Spec.NodeSelector,
		},
	}

	// can't conditionalize the manage-dockerfile init container because we don't
	// know until we've cloned, whether or not we've got a dockerfile to manage
	// (also if it's a docker type build, we should always have a dockerfile to manage)
	if build.Spec.Source.Git != nil || build.Spec.Source.Binary != nil {
		gitCloneContainer := v1.Container{
			Name:    GitCloneContainer,
			Image:   bs.Image,
			Command: []string{"openshift-git-clone"},
			Env:     copyEnvVarSlice(containerEnv),
			TerminationMessagePolicy: v1.TerminationMessageFallbackToLogsOnError,
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "buildworkdir",
					MountPath: buildutil.BuildWorkDirMount,
				},
			},
			ImagePullPolicy: v1.PullIfNotPresent,
			Resources:       buildutil.CopyApiResourcesToV1Resources(&build.Spec.Resources),
		}
		if build.Spec.Source.Binary != nil {
			gitCloneContainer.Stdin = true
			gitCloneContainer.StdinOnce = true
		}
		setupSourceSecrets(pod, &gitCloneContainer, build.Spec.Source.SourceSecret)
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, gitCloneContainer)
	}
	if len(build.Spec.Source.Images) > 0 {
		extractImageContentContainer := v1.Container{
			Name:    ExtractImageContentContainer,
			Image:   bs.Image,
			Command: []string{"openshift-extract-image-content"},
			Env:     copyEnvVarSlice(containerEnv),
			// TODO: run unprivileged https://github.com/openshift/origin/issues/662
			SecurityContext: &v1.SecurityContext{
				Privileged: &privileged,
			},
			TerminationMessagePolicy: v1.TerminationMessageFallbackToLogsOnError,
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "buildworkdir",
					MountPath: buildutil.BuildWorkDirMount,
				},
			},
			ImagePullPolicy: v1.PullIfNotPresent,
			Resources:       buildutil.CopyApiResourcesToV1Resources(&build.Spec.Resources),
		}
		setupDockerSecrets(pod, &extractImageContentContainer, build.Spec.Output.PushSecret, strategy.PullSecret, build.Spec.Source.Images)
		pod.Spec.InitContainers = append(pod.Spec.InitContainers, extractImageContentContainer)
	}
	pod.Spec.InitContainers = append(pod.Spec.InitContainers,
		v1.Container{
			Name:    "manage-dockerfile",
			Image:   bs.Image,
			Command: []string{"openshift-manage-dockerfile"},
			Env:     copyEnvVarSlice(containerEnv),
			TerminationMessagePolicy: v1.TerminationMessageFallbackToLogsOnError,
			VolumeMounts: []v1.VolumeMount{
				{
					Name:      "buildworkdir",
					MountPath: buildutil.BuildWorkDirMount,
				},
			},
			ImagePullPolicy: v1.PullIfNotPresent,
			Resources:       buildutil.CopyApiResourcesToV1Resources(&build.Spec.Resources),
		},
	)

	if build.Spec.CompletionDeadlineSeconds != nil {
		pod.Spec.ActiveDeadlineSeconds = build.Spec.CompletionDeadlineSeconds
	}

	setOwnerReference(pod, build)
	setupDockerSecrets(pod, &pod.Spec.Containers[0], build.Spec.Output.PushSecret, strategy.PullSecret, build.Spec.Source.Images)
	// For any secrets the user wants to reference from their Assemble script or Dockerfile, mount those
	// secrets into the main container.  The main container includes logic to copy them from the mounted
	// location into the working directory.
	// TODO: consider moving this into the git-clone container and doing the secret copying there instead.
	setupInputSecrets(pod, &pod.Spec.Containers[0], build.Spec.Source.Secrets)
	setupInputConfigMaps(pod, &pod.Spec.Containers[0], build.Spec.Source.ConfigMaps)
	return pod, nil
}
