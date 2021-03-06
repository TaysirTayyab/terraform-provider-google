package google

import (
	"github.com/hashicorp/terraform/helper/schema"
	"google.golang.org/api/cloudresourcemanager/v1"
	"log"
)

var iamBindingSchema = map[string]*schema.Schema{
	"role": {
		Type:     schema.TypeString,
		Required: true,
		ForceNew: true,
	},
	"members": {
		Type:     schema.TypeSet,
		Required: true,
		Elem: &schema.Schema{
			Type: schema.TypeString,
		},
	},
	"etag": {
		Type:     schema.TypeString,
		Computed: true,
	},
}

func ResourceIamBinding(parentSpecificSchema map[string]*schema.Schema, newUpdaterFunc newResourceIamUpdaterFunc) *schema.Resource {
	return &schema.Resource{
		Create: resourceIamBindingCreate(newUpdaterFunc),
		Read:   resourceIamBindingRead(newUpdaterFunc),
		Update: resourceIamBindingUpdate(newUpdaterFunc),
		Delete: resourceIamBindingDelete(newUpdaterFunc),

		Schema: mergeSchemas(iamBindingSchema, parentSpecificSchema),
	}
}

func resourceIamBindingCreate(newUpdaterFunc newResourceIamUpdaterFunc) schema.CreateFunc {
	return func(d *schema.ResourceData, meta interface{}) error {
		config := meta.(*Config)
		updater, err := newUpdaterFunc(d, config)
		if err != nil {
			return err
		}

		p := getResourceIamBinding(d)
		err = iamPolicyReadModifyWrite(updater, func(ep *cloudresourcemanager.Policy) error {
			// Creating a binding does not remove existing members if they are not in the provided members list.
			// This prevents removing existing permission without the user's knowledge.
			// Instead, a diff is shown in that case after creation. Subsequent calls to update will remove any
			// existing members not present in the provided list.
			ep.Bindings = mergeBindings(append(ep.Bindings, p))
			return nil
		})
		if err != nil {
			return err
		}
		d.SetId(updater.GetResourceId() + "/" + p.Role)
		return resourceIamBindingRead(newUpdaterFunc)(d, meta)
	}
}

func resourceIamBindingRead(newUpdaterFunc newResourceIamUpdaterFunc) schema.ReadFunc {
	return func(d *schema.ResourceData, meta interface{}) error {
		config := meta.(*Config)
		updater, err := newUpdaterFunc(d, config)
		if err != nil {
			return err
		}

		eBinding := getResourceIamBinding(d)
		p, err := updater.GetResourceIamPolicy()
		if err != nil {
			return err
		}
		log.Printf("[DEBUG]: Retrieved policy for %s: %+v\n", updater.DescribeResource(), p)

		var binding *cloudresourcemanager.Binding
		for _, b := range p.Bindings {
			if b.Role != eBinding.Role {
				continue
			}
			binding = b
			break
		}
		if binding == nil {
			log.Printf("[DEBUG]: Binding for role %q not found in policy for %s, removing from state file.\n", eBinding.Role, updater.DescribeResource())
			d.SetId("")
			return nil
		}
		d.Set("etag", p.Etag)
		d.Set("members", binding.Members)
		d.Set("role", binding.Role)
		return nil
	}
}

func resourceIamBindingUpdate(newUpdaterFunc newResourceIamUpdaterFunc) schema.UpdateFunc {
	return func(d *schema.ResourceData, meta interface{}) error {
		config := meta.(*Config)
		updater, err := newUpdaterFunc(d, config)
		if err != nil {
			return err
		}

		binding := getResourceIamBinding(d)
		err = iamPolicyReadModifyWrite(updater, func(p *cloudresourcemanager.Policy) error {
			var found bool
			for pos, b := range p.Bindings {
				if b.Role != binding.Role {
					continue
				}
				found = true
				p.Bindings[pos] = binding
				break
			}
			if !found {
				p.Bindings = append(p.Bindings, binding)
			}
			return nil
		})
		if err != nil {
			return err
		}

		return resourceIamBindingRead(newUpdaterFunc)(d, meta)
	}
}

func resourceIamBindingDelete(newUpdaterFunc newResourceIamUpdaterFunc) schema.DeleteFunc {
	return func(d *schema.ResourceData, meta interface{}) error {
		config := meta.(*Config)
		updater, err := newUpdaterFunc(d, config)
		if err != nil {
			return err
		}

		binding := getResourceIamBinding(d)
		err = iamPolicyReadModifyWrite(updater, func(p *cloudresourcemanager.Policy) error {
			toRemove := -1
			for pos, b := range p.Bindings {
				if b.Role != binding.Role {
					continue
				}
				toRemove = pos
				break
			}
			if toRemove < 0 {
				log.Printf("[DEBUG]: Policy bindings for %s did not include a binding for role %q", updater.DescribeResource(), binding.Role)
				return nil
			}

			p.Bindings = append(p.Bindings[:toRemove], p.Bindings[toRemove+1:]...)
			return nil
		})
		if err != nil {
			return err
		}

		return resourceIamBindingRead(newUpdaterFunc)(d, meta)
	}
}

func getResourceIamBinding(d *schema.ResourceData) *cloudresourcemanager.Binding {
	members := d.Get("members").(*schema.Set).List()
	return &cloudresourcemanager.Binding{
		Members: convertStringArr(members),
		Role:    d.Get("role").(string),
	}
}
