// Code generated by mdatagen. DO NOT EDIT.

package metadata

import "go.opentelemetry.io/collector/confmap"

// ResourceAttributeConfig provides common config for a particular resource attribute.
type ResourceAttributeConfig struct {
	Enabled bool `mapstructure:"enabled"`

	enabledSetByUser bool
}

func (rac *ResourceAttributeConfig) Unmarshal(parser *confmap.Conf) error {
	if parser == nil {
		return nil
	}
	err := parser.Unmarshal(rac, confmap.WithErrorUnused())
	if err != nil {
		return err
	}
	rac.enabledSetByUser = parser.IsSet("enabled")
	return nil
}

// ResourceAttributesConfig provides config for resourcedetectionprocessor/elastic_beanstalk resource attributes.
type ResourceAttributesConfig struct {
	CloudPlatform         ResourceAttributeConfig `mapstructure:"cloud.platform"`
	CloudProvider         ResourceAttributeConfig `mapstructure:"cloud.provider"`
	DeploymentEnvironment ResourceAttributeConfig `mapstructure:"deployment.environment"`
	ServiceInstanceID     ResourceAttributeConfig `mapstructure:"service.instance.id"`
	ServiceVersion        ResourceAttributeConfig `mapstructure:"service.version"`
}

func DefaultResourceAttributesConfig() ResourceAttributesConfig {
	return ResourceAttributesConfig{
		CloudPlatform: ResourceAttributeConfig{
			Enabled: true,
		},
		CloudProvider: ResourceAttributeConfig{
			Enabled: true,
		},
		DeploymentEnvironment: ResourceAttributeConfig{
			Enabled: true,
		},
		ServiceInstanceID: ResourceAttributeConfig{
			Enabled: true,
		},
		ServiceVersion: ResourceAttributeConfig{
			Enabled: true,
		},
	}
}
