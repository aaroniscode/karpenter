/*
Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1alpha1

import (
	"context"
	"fmt"
	"net/url"

	"github.com/awslabs/karpenter/pkg/apis/provisioning/v1alpha4"
	"knative.dev/pkg/apis"
)

func (c *Constraints) Validate(ctx context.Context) (errs *apis.FieldError) {
	return c.validate(ctx).ViaField("provider")
}

func (c *Constraints) validate(ctx context.Context) (errs *apis.FieldError) {
	return errs.Also(
		c.validateInstanceProfile(),
		c.validateCapacityTypes(),
		c.validateLaunchTemplate(),
		c.validateSubnets(),
		c.validateSecurityGroups(),
		c.Cluster.Validate(ctx).ViaField("cluster"),
	)
}

func (c *Constraints) validateCapacityTypes() (errs *apis.FieldError) {
	return v1alpha4.ValidateWellKnown(CapacityTypeLabel, c.CapacityTypes, "capacityTypes")
}

func (c *Constraints) validateInstanceProfile() (errs *apis.FieldError) {
	if c.InstanceProfile == "" {
		errs = errs.Also(apis.ErrMissingField("instanceProfile"))
	}
	return errs
}

func (c *Constraints) validateLaunchTemplate() (errs *apis.FieldError) {
	// nothing to validate at the moment
	return errs
}

func (c *Constraints) validateSubnets() (errs *apis.FieldError) {
	if c.SubnetSelector == nil {
		errs = errs.Also(apis.ErrMissingField("subnetSelector"))
	}
	for key, value := range c.SubnetSelector {
		if key == "" || value == "" {
			errs = errs.Also(apis.ErrInvalidValue("\"\"", fmt.Sprintf("subnetSelector['%s']", key)))
		}
	}
	return errs
}

func (c *Constraints) validateSecurityGroups() (errs *apis.FieldError) {
	if c.SecurityGroupSelector == nil {
		errs = errs.Also(apis.ErrMissingField("securityGroupSelector"))
	}
	for key, value := range c.SecurityGroupSelector {
		if key == "" || value == "" {
			errs = errs.Also(apis.ErrInvalidValue("\"\"", fmt.Sprintf("securityGroupSelector['%s']", key)))
		}
	}
	return errs
}

func (c *Cluster) Validate(context.Context) (errs *apis.FieldError) {
	if len(c.Name) == 0 {
		errs = errs.Also(apis.ErrMissingField("name"))
	}
	if len(c.Endpoint) == 0 {
		errs = errs.Also(apis.ErrMissingField("endpoint"))
	} else {
		endpoint, err := url.Parse(c.Endpoint)
		// url.Parse() will accept a lot of input without error; make
		// sure it's a real URL
		if err != nil || !endpoint.IsAbs() || endpoint.Hostname() == "" {
			errs = errs.Also(apis.ErrInvalidValue(fmt.Sprintf("%s not a valid URL", c.Endpoint), "endpoint"))
		}
	}
	return errs
}
