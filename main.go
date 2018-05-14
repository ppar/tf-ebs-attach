package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/docopt/docopt-go"
	"github.com/hashicorp/terraform/helper/hashcode"
	"io/ioutil"
	"os"
)

//       1         2         3         4         5         6         7         8
//345678901234567890123456789012345678901234567890123456789012345678901234567890
const usage = `terraform-ebs-attachment

Usage:
  tf-ebs-attach import [-i f] [-o f] <inst-name> <vol-name> <att-name> <dev>  
  tf-ebs-attach show <inst-id> <vol-name> <vol-id> <att-name> <dev>
  tf-ebs-attach --help
  
This tool lets you "import" an AWS EBS volume attachment into your Terraform 
state file. While Terraform lets you import AWS instances and EBS volumes, it 
does not support importing the synthetic "aws_volume_attachment" resource that 
has no identifiable counterpart in AWS.  

Options:
  -i file Read existing Terraform state from "file" [default: terraform.tfstate]
  -o file Write updated Terraform state to "file" [default: terraform.tfstate]

  inst-name: Name of the "aws_instance"          resource in your Terraform code 
  vol-name:  Name of the "aws_ebs_volume"        resource in your Terraform code
  att-name:  Name of the "aws_volume_attachment" resource in your Terraform code
  
  inst-id:   EC2 Instance ID (i-abcd123)
  vol-id:    EBS Volume ID (vol-abcd123)
  
  dev:      Value of "device_name" from "aws_volume_attachment"

Modes:
  import: Reads in a terraform state file, locates the definitions for 
          <inst-name> and <vol-name> and injects a new definition for the volume 
          attachment <vol-name>. 
  show:   Prints out the resource object that would be inserted given the 
          specified instance and volume. Doesn't use a terraform state file. 

Examples:
  tf-ebs-attach import mysrv mysrv_dsk0 mysrv_dsk0_attch /dev/sdg
  tf-ebs-attach show i-abc123 mysrv_dsk0 vol-123abc mysrv_dsk0_att /dev/sdg
  
To get a useful diff, normalize with jq:
  jq -S . < terraform.tfstate > old.tmp 
  tf-ebs-attach import -i old.tmp -o - [...] | jq -S . | diff -u old.tmp -
`

func main() {
	opts, err := docopt.ParseDoc(usage)
	if err != nil {
		die("Internal error parsing docopt string: %s", err)
	}

	switch os.Args[1] {
	case "show":
		showMode(opts)
	case "import":
		importMode(opts)
	}
}

func die(message string, err error) {
	if err != nil {
		fmt.Printf(message+"\n", err)
	} else {
		fmt.Printf(message + "\n")
	}
	os.Exit(1)
}

func showMode(opts docopt.Opts) {
	instanceID := getOpt(opts, "<inst-id>")
	volumeName := getOpt(opts, "<vol-name>")
	volumeID := getOpt(opts, "<vol-id>")
	attachmentName := getOpt(opts, "<att-name>")
	deviceName := getOpt(opts, "<dev>")

	result := make(map[string]interface{})
	result["aws_volume_attachment."+attachmentName] =
		newAwsVolumeAttachmentState(instanceID, volumeName, volumeID, deviceName)

	outputData, err := json.MarshalIndent(result, "", "    ")
	if err != nil {
		die("Error encoding output to JSON: %s", err)
	}

	fmt.Print(string(outputData) + "\n")
}

func importMode(opts docopt.Opts) {
	// Contents of the terraform state file
	var tfstate map[string]interface{}

	// Parse options
	inputFileName := getOpt(opts, "-i")
	outputFileName := getOpt(opts, "-o")
	instanceName := getOpt(opts, "<inst-name>")
	volumeName := getOpt(opts, "<vol-name>")
	attachmentName := getOpt(opts, "<att-name>")
	deviceName := getOpt(opts, "<dev>")

	if inputFileName == "-" {
		inputFileName = "/dev/stdin"
	}
	if outputFileName == "-" {
		outputFileName = "/dev/stdout"
	}

	// Read in tfstate
	inputData, err := ioutil.ReadFile(inputFileName)
	if err != nil {
		die("Error reading input file: %s", err)
	}
	if err = json.Unmarshal(inputData, &tfstate); err != nil {
		die("Error parsing input file as JSON: %s", err)
	}

	// Locate our instance and volume
	var instanceID, volumeID string
	tmp, found := tfstate["modules"]
	if !found {
		die("Could not find \"modules\" in input", nil)
	}
	modulesSlice := tmp.([]interface{})
	var moduleIndex int
	var module interface{}
	var resources map[string]interface{}
	for moduleIndex, module = range modulesSlice {
		tmp, found = module.(map[string]interface{})["resources"]
		if !found {
			die("Could not find \"resources\" in modules", nil)
		}
		resources = tmp.(map[string]interface{})
		instanceID, found = getResourceID(resources, "aws_instance."+instanceName)
		if !found {
			continue
		}
		volumeID, found = getResourceID(resources, "aws_ebs_volume."+volumeName)
		if found {
			break
		}
	}
	if !found {
		die("Could not locate instance and volume in the same module block in tfstate", nil)
	}

	// Create attachment resource, inject it back to tfstate
	resources["aws_volume_attachment."+attachmentName] =
		newAwsVolumeAttachmentState(instanceID, volumeName, volumeID, deviceName)

	module.(map[string]interface{})["resources"] = resources
	modulesSlice[moduleIndex] = module
	tfstate["modules"] = modulesSlice

	// Encode and write it out tfstate
	outputData, err := json.MarshalIndent(tfstate, "", "    ")
	if err != nil {
		die("Error encoding output to JSON: %s", err)
	}
	outputData = append(outputData, []byte("\n")[0])
	err = ioutil.WriteFile(outputFileName, outputData, 0644)
	if err != nil {
		die("Error writing output file: %s", err)
	}
}

func getOpt(opts docopt.Opts, key string) string {
	val, err := opts.String(key)
	if err != nil {
		die("Error parsing option "+key+": %s", err)
	}
	return val
}

func getResourceID(resources map[string]interface{}, key string) (string, bool) {
	resource, found := resources[key]
	if !found {
		return "", false
	}

	primary, found := resource.(map[string]interface{})["primary"]
	if !found {
		return "", false
	}

	valID, found := primary.(map[string]interface{})["id"]
	if !found {
		return "", false
	}

	return valID.(string), true
}

type awsVolumeAttachmentState struct {
	Type      string                          `json:"type"`
	DependsOn []string                        `json:"depends_on"`
	Primary   awsVolumeAttachmentStatePrimary `json:"primary"`
	Deposed   []interface{}                   `json:"deposed"`
	Provider  string                          `json:"provider"`
}

type awsVolumeAttachmentStatePrimary struct {
	Id         string                 `json:"id"`
	Attributes map[string]string      `json:"attributes"`
	Meta       map[string]interface{} `json:"meta"`
	Tainted    bool                   `json:"tainted"`
}

func newAwsVolumeAttachmentState(instanceID, volumeName, volumeID, deviceName string) awsVolumeAttachmentState {
	return awsVolumeAttachmentState{
		Type: "aws_volume_attachment",
		DependsOn: []string{
			fmt.Sprintf("aws_ebs_volume.%s", volumeName),
		},
		Primary: awsVolumeAttachmentStatePrimary{
			Id: volumeAttachmentID(deviceName, volumeID, instanceID),
			Attributes: map[string]string{
				"id":          volumeAttachmentID(deviceName, volumeID, instanceID),
				"device_name": deviceName,
				"instance_id": instanceID,
				"volume_id":   volumeID,
			},
			Meta:    make(map[string]interface{}),
			Tainted: false,
		},
		Deposed: []interface{}{},
	}
}

// From https://github.com/foxsy/tfvolattid/blob/master/tfvolattid.go
func volumeAttachmentID(name, volumeID, instanceID string) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%s-", name))
	buf.WriteString(fmt.Sprintf("%s-", instanceID))
	buf.WriteString(fmt.Sprintf("%s-", volumeID))

	return fmt.Sprintf("vai-%d", hashcode.String(buf.String()))
}
