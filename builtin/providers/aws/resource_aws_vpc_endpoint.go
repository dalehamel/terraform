package aws

import (
	"fmt"
	"log"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ec2"
	"github.com/hashicorp/terraform/helper/schema"
)

func resourceAwsVpcEndpoint() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsVPCEndpointCreate,
		Read:   resourceAwsVPCEndpointRead,
		Update: resourceAwsVPCEndpointUpdate,
		Delete: resourceAwsVPCEndpointDelete,

		Schema: map[string]*schema.Schema{
			"vpc_id": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"service_name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},

			"route_tables": &schema.Schema{
				Type:     schema.TypeList,
				Required: true,
        Computed: true,
				Elem:     &schema.Schema{Type: schema.TypeString},
			},

			"policy_document": &schema.Schema{
				Type:     schema.TypeString,
        Computed: true,
				Optional: true,
			},

      // computed
			"state": &schema.Schema{
				Type:     schema.TypeString,
        Computed: true,
			},


//(pending | available | deleting | deleted )
		},
	}
}

func resourceAwsVPCEndpointCreate(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ec2conn

	// Create the vpc endpoint
	createOpts := &ec2.CreateVPCEndpointInput{
		VPCID:             aws.String(d.Get("vpc_id").(string)),
		ServiceName:       aws.String(d.Get("service_name").(string)),
	}

  if raw, ok := d.GetOk("route_tables"); ok {
    list := raw.([]interface{})
    var route_tables []*string
    for i, v := range list {
      route_tables[i] = aws.String(v.(string))
    }
    createOpts.RouteTableIDs = route_tables
  }

	if v := d.Get("policy_document"); v != nil {
		createOpts.PolicyDocument = aws.String(v.(string))
	}

	log.Printf("[DEBUG] VPCEndpointCreate create config: %#v", createOpts)
	resp, err := conn.CreateVPCEndpoint(createOpts)
	if err != nil {
		return fmt.Errorf("Error creating vpc endpoint: %s", err)
	}

  // FIXME poll for completed

	// Get the ID and store it
	rt := resp.VPCEndpoint
	d.SetId(*rt.VPCEndpointID)
	log.Printf("[INFO] VPC Endpoint ID: %s", d.Id())

	return resourceAwsVPCEndpointUpdate(d, meta)
}

func resourceAwsVPCEndpointRead(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ec2conn

  readOpts := &ec2.DescribeVPCEndpointsInput {
    VPCEndpointIDs:  []*string{aws.String(d.Id())},
  }

  resp, err := conn.DescribeVPCEndpoints(readOpts)
	if err != nil {
		return fmt.Errorf("Error reading vpc endpoint %s", err)
	}

  if len(resp.VPCEndpoints) > 0 {
    vpc_endpoint := resp.VPCEndpoints[0]

    d.Set("state", vpc_endpoint.State)
    d.Set("policy_document", vpc_endpoint.PolicyDocument)
    d.Set("route_tables", vpc_endpoint.RouteTableIDs)

  } else {
    d.Set("state", "")
    d.Set("policy_document", "")
    d.Set("route_tables", [...]string{"", "Teller"})
  }
	return nil
}


func resourceAwsVPCEndpointUpdate(d *schema.ResourceData, meta interface{}) error {
/*
	conn := meta.(*AWSClient).ec2conn
  updateOpts := &ec2.ModifyVPCEndpointInput {
    VPCEndpointID: aws.String(d.Id()),
  }
*/

// FIXME: compute if routes should be added or removed

// AddRouteTableIDs
// RemoveRouteTableIDs

// FIXME: check if policy document changed
// PolicyDocument

	return resourceAwsVPCEndpointRead(d, meta)
}

func resourceAwsVPCEndpointDelete(d *schema.ResourceData, meta interface{}) error {
	conn := meta.(*AWSClient).ec2conn
  deleteOpts := &ec2.DeleteVPCEndpointsInput {
    VPCEndpointIDs:  []*string{aws.String(d.Id())},
  }
	_, err := conn.DeleteVPCEndpoints(deleteOpts)

	return err
}
