package aws

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"log"

	"github.com/hashicorp/terraform/helper/schema"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/sns"
)

var SupportedPlatforms = map[string]bool{
	"ADM":          true,  // (Amazon Device Messaging)
	"APNS":         true,  // (Apple Push Notification Service)
	"APNS_SANDBOX": true,  // (Apple Push Notification Service)
	"GCM":          false, // (Google Cloud Messaging).
}

// Mutable attributes
// http://docs.aws.amazon.com/sns/latest/api/API_SetPlatformApplicationAttributes.html
var SNSPlatformAppAttributeMap = map[string]string{
	"principal":           "PlatformPrincipal",
	"created_topic":       "EventEndpointCreated",
	"deleted_topic":       "EventEndpointDeleted",
	"updated_topic":       "EventEndpointUpdated",
	"failure_topic":       "EventDeliveryFailure",
	"success_iam_arn":     "SuccessFeedbackRoleArn",
	"failure_iam_arn":     "FailureFeedbackRoleArn",
	"success_sample_rate": "SuccessFeedbackSampleRate",
}

func resourceAwsSnsApplication() *schema.Resource {
	return &schema.Resource{
		Create: resourceAwsSnsApplicationCreate,
		Read:   resourceAwsSnsApplicationRead,
		Update: resourceAwsSnsApplicationUpdate,
		Delete: resourceAwsSnsApplicationDelete,

		Schema: map[string]*schema.Schema{
			"name": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"platform": &schema.Schema{
				Type:     schema.TypeString,
				Required: true,
				ForceNew: true,
			},
			"credential": &schema.Schema{
				Type:      schema.TypeString,
				Required:  true,
				ForceNew:  false,
				StateFunc: hashSum,
			},
			"principal": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"created_topic": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"deleted_topic": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"updated_topic": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"failure_topic": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"success_iam_role": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"failure_iam_role": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"success_sample_rate": &schema.Schema{
				Type:     schema.TypeString,
				Optional: true,
				ForceNew: false,
			},
			"arn": &schema.Schema{
				Type:     schema.TypeString,
				Computed: true,
			},
		},
	}
}

func resourceAwsSnsApplicationCreate(d *schema.ResourceData, meta interface{}) error {
	snsconn := meta.(*AWSClient).snsconn

	attributes := make(map[string]*string)
	name := d.Get("name").(string)
	platform := d.Get("platform").(string)
	principal := d.Get("principal").(string)

	attributes["PlatformCredential"] = aws.String(d.Get("credential").(string))

	if _, ok := SupportedPlatforms[platform]; !ok {
		return errors.New(fmt.Sprintf("Platform %s is not supported", platform))
	}

	if value, _ := SupportedPlatforms[platform]; value {
		if principal == "" {
			return errors.New(fmt.Sprintf("Principal is required for %s", platform))
		} else {
			attributes["PlatformPrincipal"] = aws.String(principal)
		}
	}

	log.Printf("[DEBUG] SNS create application: %s", name)

	req := &sns.CreatePlatformApplicationInput{
		Name:       aws.String(name),
		Platform:   aws.String(platform),
		Attributes: attributes,
	}

	output, err := snsconn.CreatePlatformApplication(req)
	if err != nil {
		return fmt.Errorf("Error creating SNS application: %s", err)
	}

	d.SetId(*output.PlatformApplicationArn)

	// Write the ARN to the 'arn' field for export
	d.Set("arn", *output.PlatformApplicationArn)

	return resourceAwsSnsApplicationUpdate(d, meta)
}

func resourceAwsSnsApplicationUpdate(d *schema.ResourceData, meta interface{}) error {
	snsconn := meta.(*AWSClient).snsconn

	resource := *resourceAwsSnsApplication()

	attributes := make(map[string]*string)

	for k, _ := range resource.Schema {
		if attrKey, ok := SNSPlatformAppAttributeMap[k]; ok {
			if d.HasChange(k) {
				log.Printf("[DEBUG] Updating %s", attrKey)
				_, n := d.GetChange(k)
				attributes[attrKey] = aws.String(n.(string))
			}
		}
	}

	if d.HasChange("credential") {
		attributes["PlatformCredential"] = aws.String(d.Get("credential").(string))
		// If the platform requires a principal it must also be specified, even if it didn't change
		// since credential is stored as a hash, the only way to update principal is to update both
		// as they must be specified together in the request.
		if v, _ := SupportedPlatforms[d.Get("platform").(string)]; v {
			attributes["PlatformPrincipal"] = aws.String(d.Get("principal").(string))
		}
	}

	// Make API call to update attributes
	req := &sns.SetPlatformApplicationAttributesInput{
		PlatformApplicationArn: aws.String(d.Id()),
		Attributes:             attributes,
	}
	_, err := snsconn.SetPlatformApplicationAttributes(req)

	if err != nil {
		return fmt.Errorf("Error updating SNS application: %s", err)
	}

	return resourceAwsSnsApplicationRead(d, meta)
}

func resourceAwsSnsApplicationRead(d *schema.ResourceData, meta interface{}) error {
	snsconn := meta.(*AWSClient).snsconn

	attributeOutput, err := snsconn.GetPlatformApplicationAttributes(&sns.GetPlatformApplicationAttributesInput{
		PlatformApplicationArn: aws.String(d.Id()),
	})

	if err != nil {
		return err
	}

	if attributeOutput.Attributes != nil && len(attributeOutput.Attributes) > 0 {
		attrmap := attributeOutput.Attributes
		resource := *resourceAwsSnsApplication()
		// iKey = internal struct key, oKey = AWS Attribute Map key
		for iKey, oKey := range SNSPlatformAppAttributeMap {
			log.Printf("[DEBUG] Updating %s => %s", iKey, oKey)

			if attrmap[oKey] != nil {
				// Some of the fetched attributes are stateful properties such as
				// the number of subscriptions, the owner, etc. skip those
				if resource.Schema[iKey] != nil {
					value := *attrmap[oKey]
					log.Printf("[DEBUG] Updating %s => %s -> %s", iKey, oKey, value)
					d.Set(iKey, *attrmap[oKey])
				}
			}
		}
	}

	return nil
}

func resourceAwsSnsApplicationDelete(d *schema.ResourceData, meta interface{}) error {
	snsconn := meta.(*AWSClient).snsconn

	log.Printf("[DEBUG] SNS Delete Application: %s", d.Id())
	_, err := snsconn.DeletePlatformApplication(&sns.DeletePlatformApplicationInput{
		PlatformApplicationArn: aws.String(d.Id()),
	})
	if err != nil {
		return err
	}
	return nil
}

func hashSum(contents interface{}) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(contents.(string))))
}