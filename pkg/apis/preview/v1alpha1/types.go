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

	Spec PreviewSpec `json:"spec,omitempty"`
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

// Resources represents details of the preview application
type Resources struct {
	// Name the name of the preview if different from the repository name
	Name string `json:"name,omitempty" protobuf:"bytes,1,opt,name=name"`

	// URL the URL to test out the preview if applicable
	URL string `json:"url,omitempty" protobuf:"bytes,2,opt,name=url"`

	// Namespace the optional namespace unique for the pull request to deploy into
	Namespace string `json:"namespace,omitempty" protobuf:"bytes,3,opt,name=namespace"`
}

// PreviewSpec the spec of a pipeline request
type PreviewSpec struct {
	// Source the source of the pull request
	Source PreviewSource `json:"source,omitempty" protobuf:"bytes,1,opt,name=source"`

	// PullRequest the pull request which triggered it
	PullRequest PullRequest `json:"pullRequest,omitempty" protobuf:"bytes,2,opt,name=pullRequest"`

	// Resources information about the deployed resources
	Resources Resources `json:"resources,omitempty" protobuf:"bytes,3,opt,name=resources"`

	// DestroyCommand the command to destroy the preview
	DestroyCommand Command `json:"destroyCommand,omitempty" protobuf:"bytes,4,opt,name=destroyCommand"`
}

// PreviewSource the location of the preview
type PreviewSource struct {
	// URL the git URL of the source
	URL string `json:"url,omitempty" protobuf:"bytes,1,opt,name=url"`

	// CloneURL the git URL to clone the source which should include user and password
	// so that the garbage collection jobs can properly clone the repository
	CloneURL string `json:"cloneURL,omitempty" protobuf:"bytes,2,opt,name=cloneURL"`

	// Ref the git reference (sha / branch / tag) to clone the source
	Ref string `json:"ref,omitempty" protobuf:"bytes,3,opt,name=ref"`

	// Path the location of the helmfile.yaml file (defaults to charts/preview/helmfile.yaml)
	Path string `json:"path,omitempty" protobuf:"bytes,4,opt,name=path"`
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
