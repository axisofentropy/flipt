package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/viper"
	"go.flipt.io/flipt/internal/oci"
)

// cheers up the unparam linter
var _ defaulter = (*StorageConfig)(nil)

type StorageType string

const (
	DatabaseStorageType = StorageType("database")
	LocalStorageType    = StorageType("local")
	GitStorageType      = StorageType("git")
	ObjectStorageType   = StorageType("object")
	OCIStorageType      = StorageType("oci")
)

type ObjectSubStorageType string

const (
	S3ObjectSubStorageType     = ObjectSubStorageType("s3")
	AZBlobObjectSubStorageType = ObjectSubStorageType("azblob")
)

// StorageConfig contains fields which will configure the type of backend in which Flipt will serve
// flag state.
type StorageConfig struct {
	Type     StorageType `json:"type,omitempty" mapstructure:"type" yaml:"type,omitempty"`
	Local    *Local      `json:"local,omitempty" mapstructure:"local,omitempty" yaml:"local,omitempty"`
	Git      *Git        `json:"git,omitempty" mapstructure:"git,omitempty" yaml:"git,omitempty"`
	Object   *Object     `json:"object,omitempty" mapstructure:"object,omitempty" yaml:"object,omitempty"`
	OCI      *OCI        `json:"oci,omitempty" mapstructure:"oci,omitempty" yaml:"oci,omitempty"`
	ReadOnly *bool       `json:"readOnly,omitempty" mapstructure:"read_only,omitempty" yaml:"read_only,omitempty"`
}

func (c *StorageConfig) setDefaults(v *viper.Viper) error {
	switch v.GetString("storage.type") {
	case string(LocalStorageType):
		v.SetDefault("storage.local.path", ".")
	case string(GitStorageType):
		v.SetDefault("storage.git.ref", "main")
		v.SetDefault("storage.git.poll_interval", "30s")
		v.SetDefault("storage.git.insecure_skip_tls", false)
		if v.GetString("storage.git.authentication.ssh.password") != "" ||
			v.GetString("storage.git.authentication.ssh.private_key_path") != "" ||
			v.GetString("storage.git.authentication.ssh.private_key_bytes") != "" {
			v.SetDefault("storage.git.authentication.ssh.user", "git")
		}
	case string(ObjectStorageType):
		// keep this as a case statement in anticipation of
		// more object types in the future
		// nolint:gocritic
		switch v.GetString("storage.object.type") {
		case string(S3ObjectSubStorageType):
			v.SetDefault("storage.object.s3.poll_interval", "1m")
		case string(AZBlobObjectSubStorageType):
			v.SetDefault("storage.object.azblob.poll_interval", "1m")
		}
	case string(OCIStorageType):
		v.SetDefault("storage.oci.poll_interval", "30s")

		dir, err := DefaultBundleDir()
		if err != nil {
			return err
		}

		v.SetDefault("storage.oci.bundles_directory", dir)
	default:
		v.SetDefault("storage.type", "database")
	}

	return nil
}

func (c *StorageConfig) validate() error {
	switch c.Type {
	case GitStorageType:
		if c.Git.Ref == "" {
			return errors.New("git ref must be specified")
		}
		if c.Git.Repository == "" {
			return errors.New("git repository must be specified")
		}

		if err := c.Git.Authentication.validate(); err != nil {
			return err
		}
		if err := c.Git.validate(); err != nil {
			return err
		}

	case LocalStorageType:
		if c.Local.Path == "" {
			return errors.New("local path must be specified")
		}

	case ObjectStorageType:
		if c.Object == nil {
			return errors.New("object storage type must be specified")
		}
		if err := c.Object.validate(); err != nil {
			return err
		}
	case OCIStorageType:
		if c.OCI.Repository == "" {
			return errors.New("oci storage repository must be specified")
		}

		if _, err := oci.ParseReference(c.OCI.Repository); err != nil {
			return fmt.Errorf("validating OCI configuration: %w", err)
		}
	}

	// setting read only mode is only supported with database storage
	if c.ReadOnly != nil && !*c.ReadOnly && c.Type != DatabaseStorageType {
		return errors.New("setting read only mode is only supported with database storage")
	}

	return nil
}

// Local contains configuration for referencing a local filesystem.
type Local struct {
	Path string `json:"path,omitempty" mapstructure:"path"`
}

// Git contains configuration for referencing a git repository.
type Git struct {
	Repository      string         `json:"repository,omitempty" mapstructure:"repository" yaml:"repository,omitempty"`
	Ref             string         `json:"ref,omitempty" mapstructure:"ref" yaml:"ref,omitempty"`
	CaCertBytes     string         `json:"-" mapstructure:"ca_cert_bytes" yaml:"-" `
	CaCertPath      string         `json:"-" mapstructure:"ca_cert_path" yaml:"-" `
	InsecureSkipTLS bool           `json:"-" mapstructure:"insecure_skip_tls" yaml:"-"`
	PollInterval    time.Duration  `json:"pollInterval,omitempty" mapstructure:"poll_interval" yaml:"poll_interval,omitempty"`
	Authentication  Authentication `json:"-" mapstructure:"authentication,omitempty" yaml:"-"`
}

func (g *Git) validate() error {
	if g.CaCertPath != "" && g.CaCertBytes != "" {
		return errors.New("please provide only one of ca_cert_path or ca_cert_bytes")
	}
	return nil
}

// Object contains configuration of readonly object storage.
type Object struct {
	Type   ObjectSubStorageType `json:"type,omitempty" mapstructure:"type" yaml:"type,omitempty"`
	S3     *S3                  `json:"s3,omitempty" mapstructure:"s3,omitempty" yaml:"s3,omitempty"`
	AZBlob *AZBlob              `json:"azblob,omitempty" mapstructure:"azblob,omitempty" yaml:"azblob,omitempty"`
}

// validate is only called if storage.type == "object"
func (o *Object) validate() error {
	switch o.Type {
	case S3ObjectSubStorageType:
		if o.S3 == nil || o.S3.Bucket == "" {
			return errors.New("s3 bucket must be specified")
		}
	case AZBlobObjectSubStorageType:
		if o.AZBlob == nil || o.AZBlob.Container == "" {
			return errors.New("azblob container must be specified")
		}
	default:
		return errors.New("object storage type must be specified")
	}
	return nil
}

// S3 contains configuration for referencing a s3 bucket
type S3 struct {
	Endpoint     string        `json:"endpoint,omitempty" mapstructure:"endpoint" yaml:"endpoint,omitempty"`
	Bucket       string        `json:"bucket,omitempty" mapstructure:"bucket" yaml:"bucket,omitempty"`
	Prefix       string        `json:"prefix,omitempty" mapstructure:"prefix" yaml:"prefix,omitempty"`
	Region       string        `json:"region,omitempty" mapstructure:"region" yaml:"region,omitempty"`
	PollInterval time.Duration `json:"pollInterval,omitempty" mapstructure:"poll_interval" yaml:"poll_interval,omitempty"`
}

// AZBlob contains configuration for referencing a Azure Blob Storage
type AZBlob struct {
	Endpoint     string        `json:"-" mapstructure:"endpoint" yaml:"endpoint,omitempty"`
	Container    string        `json:"container,omitempty" mapstructure:"container" yaml:"container,omitempty"`
	PollInterval time.Duration `json:"pollInterval,omitempty" mapstructure:"poll_interval" yaml:"poll_interval,omitempty"`
}

// Authentication holds structures for various types of auth we support.
// Token auth will take priority over Basic auth if both are provided.
//
// To make things easier, if there are multiple inputs that a particular auth method needs, and
// not all inputs are given but only partially, we will return a validation error.
// (e.g. if username for basic auth is given, and token is also given a validation error will be returned)
type Authentication struct {
	BasicAuth *BasicAuth `json:"-" mapstructure:"basic,omitempty" yaml:"-"`
	TokenAuth *TokenAuth `json:"-" mapstructure:"token,omitempty" yaml:"-"`
	SSHAuth   *SSHAuth   `json:"-" mapstructure:"ssh,omitempty" yaml:"-"`
}

func (a *Authentication) validate() error {
	if a.BasicAuth != nil {
		if err := a.BasicAuth.validate(); err != nil {
			return err
		}
	}
	if a.TokenAuth != nil {
		if err := a.TokenAuth.validate(); err != nil {
			return err
		}
	}
	if a.SSHAuth != nil {
		if err := a.SSHAuth.validate(); err != nil {
			return err
		}
	}

	return nil
}

// BasicAuth has configuration for authenticating with private git repositories
// with basic auth.
type BasicAuth struct {
	Username string `json:"-" mapstructure:"username" yaml:"-"`
	Password string `json:"-" mapstructure:"password" yaml:"-"`
}

func (b BasicAuth) validate() error {
	if (b.Username != "" && b.Password == "") || (b.Username == "" && b.Password != "") {
		return errors.New("both username and password need to be provided for basic auth")
	}

	return nil
}

// TokenAuth has configuration for authenticating with private git repositories
// with token auth.
type TokenAuth struct {
	AccessToken string `json:"-" mapstructure:"access_token" yaml:"-"`
}

func (t TokenAuth) validate() error { return nil }

// SSHAuth provides configuration support for SSH private key credentials when
// authenticating with private git repositories
type SSHAuth struct {
	User                  string `json:"-" mapstructure:"user" yaml:"-" `
	Password              string `json:"-" mapstructure:"password" yaml:"-" `
	PrivateKeyBytes       string `json:"-" mapstructure:"private_key_bytes" yaml:"-" `
	PrivateKeyPath        string `json:"-" mapstructure:"private_key_path" yaml:"-" `
	InsecureIgnoreHostKey bool   `json:"-" mapstructure:"insecure_ignore_host_key" yaml:"-"`
}

func (a SSHAuth) validate() (err error) {
	defer func() {
		if err != nil {
			err = fmt.Errorf("ssh authentication: %w", err)
		}
	}()

	if a.Password == "" {
		return errors.New("password required")
	}

	if (a.PrivateKeyBytes == "" && a.PrivateKeyPath == "") || (a.PrivateKeyBytes != "" && a.PrivateKeyPath != "") {
		return errors.New("please provide exclusively one of private_key_bytes or private_key_path")
	}

	return nil
}

// OCI provides configuration support for OCI target registries as a backend store for Flipt.
type OCI struct {
	// Repository is the target repository and reference to track.
	// It should be in the form [<registry>/]<bundle>[:<tag>].
	// When the registry is omitted, the bundle is referenced via the local bundle store.
	// Tag defaults to 'latest' when not supplied.
	Repository string `json:"repository,omitempty" mapstructure:"repository" yaml:"repository,omitempty"`
	// BundlesDirectory is the root directory in which Flipt will store and access local feature bundles.
	BundlesDirectory string `json:"bundlesDirectory,omitempty" mapstructure:"bundles_directory" yaml:"bundles_directory,omitempty"`
	// Authentication configures authentication credentials for accessing the target registry
	Authentication *OCIAuthentication `json:"-,omitempty" mapstructure:"authentication" yaml:"-,omitempty"`
	PollInterval   time.Duration      `json:"pollInterval,omitempty" mapstructure:"poll_interval" yaml:"poll_interval,omitempty"`
}

// OCIAuthentication configures the credentials for authenticating against a target OCI regitstry
type OCIAuthentication struct {
	Username string `json:"-" mapstructure:"username" yaml:"-"`
	Password string `json:"-" mapstructure:"password" yaml:"-"`
}

func DefaultBundleDir() (string, error) {
	dir, err := Dir()
	if err != nil {
		return "", err
	}

	bundlesDir := filepath.Join(dir, "bundles")
	if err := os.MkdirAll(bundlesDir, 0755); err != nil {
		return "", fmt.Errorf("creating image directory: %w", err)
	}

	return bundlesDir, nil
}
