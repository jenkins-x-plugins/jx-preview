package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +genclient
// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=pvw

// Preview contains the definition of a preview environment
type Preview struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   PreviewSpec   `json:"spec,omitempty"`
	Status PreviewStatus `json:"status,omitempty"`
}

// +k8s:deepcopy-gen:interfaces=k8s.io/apimachinery/pkg/runtime.Object
// +kubebuilder:object:root=true

// PreviewList represents a list of pipeline options
type PreviewList struct {
	metav1.TypeMeta `json:",inline"`
	// +optional
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Preview `json:"items"`
}

// PreviewStatus represents the status of a preview
type PreviewStatus struct {
	BuildStatus     string `json:"buildStatus,omitempty" protobuf:"bytes,6,opt,name=buildStatus"`
	BuildStatusURL  string `json:"buildStatusUrl,omitempty" protobuf:"bytes,7,opt,name=buildStatusUrl"`
	ApplicationName string `json:"appName,omitempty" protobuf:"bytes,8,opt,name=appName"`
	ApplicationURL  string `json:"applicationURL,omitempty" protobuf:"bytes,9,opt,name=applicationURL"`
}

// PreviewSpec the spec of a pipeline request
type PreviewSpec struct {
	// Source the source of the pull request
	Source PreviewSource `json:"source,omitempty" protobuf:"bytes,1,opt,name=source"`

	// PullRequest the pull request which triggered it
	PullRequest PullRequest `json:"pullRequest,omitempty" protobuf:"bytes,2,opt,name=pullRequest"`

	// DestroyCommand the command to destroy the preview
	DestroyCommand Command `json:"destroyCommand,omitempty" protobuf:"bytes,3,opt,name=destroyCommand"`

	// PreviewNamespace the optional namespace unique for the pull request to deploy into
	PreviewNamespace string `json:"previewNamespace,omitempty" protobuf:"bytes,4,opt,name=previewNamespace"`
}

// PreviewSource the location of the preview
type PreviewSource struct {
	// URL the git URL to clone the source
	URL string `json:"url,omitempty" protobuf:"bytes,1,opt,name=url"`

	// Ref the git reference (sha / branch / tag) to clone the source
	Ref string `json:"ref,omitempty" protobuf:"bytes,2,opt,name=ref"`

	// Path the location of the helmfile.yaml file (defaults to charts/preview/helmfile.yaml)
	Path string `json:"path,omitempty" protobuf:"bytes,3,opt,name=ref"`
}

// PullRequest the pull request information which triggered the preview
type PullRequest struct {
	Number      int      `json:"number,omitempty" protobuf:"bytes,1,opt,name=number"`
	Owner       string   `json:"owner,omitempty" protobuf:"bytes,2,opt,name=owner"`
	Repository  string   `json:"repository,omitempty" protobuf:"bytes,3,opt,name=repository"`
	URL         string   `json:"url,omitempty" protobuf:"bytes,4,opt,name=url"`
	User        UserSpec `json:"user,omitempty" protobuf:"bytes,5,opt,name=user"`
	Title       string   `json:"title,omitempty" protobuf:"bytes,6,opt,name=title"`
	Description string   `json:"description,omitempty" protobuf:"bytes,7,opt,name=description"`
}

// UserSpec is the user details
type UserSpec struct {
	Username string `json:"username,omitempty" protobuf:"bytes,1,opt,name=username"`
	Name     string `json:"name,omitempty" protobuf:"bytes,2,opt,name=name"`
	LinkURL  string `json:"linkUrl,omitempty" protobuf:"bytes,3,opt,name=linkUrl"`
	ImageURL string `json:"imageUrl,omitempty" protobuf:"bytes,4,opt,name=imageUrl"`
}

// Command the CLI command for deleting the preview
type Command struct {
	Command string   `json:"command,omitempty" protobuf:"bytes,1,opt,name=command"`
	Args    []string `json:"args,omitempty" protobuf:"bytes,2,opt,name=args"`
	Path    string   `json:"path,omitempty" protobuf:"bytes,3,opt,name=path"`
	Env     []EnvVar `json:"env,omitempty" protobuf:"bytes,4,opt,name=env"`
}

// EnvVar the environment variable name and vlaue
type EnvVar struct {
	Name  string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`
	Value string `json:"value,omitempty" protobuf:"bytes,2,opt,name=value"`
}
