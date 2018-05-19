package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/docopt/docopt-go"
	"github.com/hashicorp/terraform/helper/hashcode"
	"github.com/hashicorp/terraform/terraform"
	"github.com/mattn/go-isatty"
	"github.com/yudai/gojsondiff"
	"github.com/yudai/gojsondiff/formatter"
	"io/ioutil"
	"os"
)

//       1         2         3         4         5         6         7         8
//345678901234567890123456789012345678901234567890123456789012345678901234567890
const usage = `terraform-ebs-attach

Usage:
  tf-ebs-attach import [-i f] [-o f] <inst-name> <vol-name> <att-name> <dev>  
  tf-ebs-attach diff   [-i f] [-c m] <inst-name> <vol-name> <att-name> <dev>  
  tf-ebs-attach show <inst-id> <vol-name> <vol-id> <att-name> <dev>
  tf-ebs-attach -h|--help
  
This tool lets you "import" an AWS EBS volume attachment into your Terraform 
state file. 

While Terraform lets you import AWS instances and EBS volumes, it doesn't seem 
to support importing the synthetic "aws_volume_attachment" resource that has no 
identifiable counterpart in AWS, so this hack provides a workaround.

Options:
  -i file Read existing Terraform state from "file" [default: terraform.tfstate]
  -o file Write updated Terraform state to "file" [default: terraform.tfstate]
  -c mode Use coloured output (mode = auto/no/yes) [default: auto]
  
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
  diff:   Prints a diff of the changes that would be made to the input file 
  show:   Prints out the resource object that would be inserted given the 
          specified instance and volume. Doesn't use a terraform state file. 

Examples:
  tf-ebs-attach import mysrv mysrv_dsk0 mysrv_dsk0_attch /dev/sdg
  tf-ebs-attach diff -i foo.state  mysrv mysrv_dsk0 mysrv_dsk0_attch /dev/sdg
  tf-ebs-attach show i-abc123 mysrv_dsk0 vol-123abc mysrv_dsk0_att /dev/sdg
`

func main() {
	opts, err := docopt.ParseDoc(usage)
	if err != nil {
		die("Internal error parsing docopt string: %s", err)
	}

	switch os.Args[1] {
	case "show":
		showMode(opts)
	case "diff":
		diffMode(opts)
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

// Show the ResourceState that would be created from the values in opts
func showMode(opts docopt.Opts) {
	instanceID, _ := opts.String("<inst-id>")
	volumeName, _ := opts.String("<vol-name>")
	volumeID, _ := opts.String("<vol-id>")
	attachmentName, _ := opts.String("<att-name>")
	deviceName, _ := opts.String("<dev>")

	result := make(map[string]*terraform.ResourceState)
	result["aws_volume_attachment."+attachmentName] =
		newAwsVolumeAttachmentState(instanceID, volumeName, volumeID, deviceName)

	outputData, err := json.MarshalIndent(result, "", "    ")
	if err != nil {
		die("Error encoding output to JSON: %s", err)
	}

	fmt.Print(string(outputData) + "\n")
}

// Show a text diff between the current tfstate ("-i") and the result of importing
// the attachment specified in opts
func diffMode(opts docopt.Opts) {
	// Read and modify tfstate
	tfstate, inputBytes := readTfState(opts)
	injectVolumeAttachment(opts, &tfstate)
	outputBytes, err := json.MarshalIndent(tfstate, "", "    ")
	if err != nil {
		die("Error encoding output to JSON: %s", err)
	}

	// Generate diff
	colors := false
	cArg, _ := opts.String("-c")
	switch cArg {
	case "":
		fallthrough
	case "auto":
		colors = isatty.IsTerminal(os.Stdout.Fd())
	case "yes":
		colors = true
	}

	diff, err := gojsondiff.New().Compare(inputBytes, outputBytes)
	if err != nil {
		die("Error comparing JSON: %s", err)
	}

	var inputJson map[string]interface{}
	err = json.Unmarshal(inputBytes, &inputJson)
	if err != nil {
		die("Error unmarshaling JSON: %s", err)
	}

	diffString, err := formatter.NewAsciiFormatter(
		inputJson,
		formatter.AsciiFormatterConfig{
			ShowArrayIndex: true,
			Coloring:       colors,
		},
	).Format(diff)
	if err != nil {
		die("Error formatting diff: %s", err)
	}

	fmt.Printf(diffString)
}

// Import the attachment specified in opts, reading from "-i", writing to "-o"
func importMode(opts docopt.Opts) {
	// Read input file
	tfstate, _ := readTfState(opts)

	// Modify it
	injectVolumeAttachment(opts, &tfstate)

	// Encode and write out tfstate
	writeTfState(opts, tfstate)
}

// Read tfstate from the file specified by "-i"
func readTfState(opts docopt.Opts) (terraform.State, []byte) {
	// Parse options
	inputFileName, _ := opts.String("-i")
	if inputFileName == "-" {
		inputFileName = "/dev/stdin"
	}
	if inputFileName == "" {
		inputFileName = "terraform.tfstate"
	}

	// Read in Terraform state
	tfstate := terraform.State{}
	inputData, err := ioutil.ReadFile(inputFileName)
	if err != nil {
		die("Error reading input file: %s", err)
	}
	if err = json.Unmarshal(inputData, &tfstate); err != nil {
		die("Error parsing input file as JSON: %s", err)
	}

	return tfstate, inputData
}

// Write out the tfstate to the file specified by "-o"
func writeTfState(opts docopt.Opts, tfstate terraform.State) {
	outputFileName, _ := opts.String("-o")
	if outputFileName == "-" {
		outputFileName = "/dev/stdout"
	}
	if outputFileName == "" {
		outputFileName = "terraform.tfstate"
	}

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

// Modify the given tfstate by adding the volume attachment specified in opts
func injectVolumeAttachment(opts docopt.Opts, tfstate *terraform.State) {
	instanceName, _ := opts.String("<inst-name>")
	volumeName, _ := opts.String("<vol-name>")
	attachmentName, _ := opts.String("<att-name>")
	deviceName, _ := opts.String("<dev>")

	// Locate our instance and volume
	instanceResourceID := "aws_instance." + instanceName
	volumeResourceID := "aws_ebs_volume." + volumeName
	for _, moduleState := range tfstate.Modules {
		//fmt.Printf("moduleState[%d]: %+v\n", i, moduleState)
		instanceState, found := moduleState.Resources[instanceResourceID]
		if !found {
			continue
		}
		volumeState, found := moduleState.Resources[volumeResourceID]
		if found {
			moduleState.Resources["aws_volume_attachment."+attachmentName] =
				newAwsVolumeAttachmentState(instanceState.Primary.ID, volumeName, volumeState.Primary.ID, deviceName)
			return
		}
	}
	die(fmt.Sprintf("Could not locate module in tfstate containing (\"%s\", \"%s\")",
		instanceResourceID, volumeResourceID), nil)
}

// Generate a new ResourceState describing our volume attachment
func newAwsVolumeAttachmentState(instanceID, volumeName, volumeID, deviceName string) *terraform.ResourceState {
	return &terraform.ResourceState{
		Type: "aws_volume_attachment",
		Dependencies: []string{
			fmt.Sprintf("aws_ebs_volume.%s", volumeName),
		},
		Primary: &terraform.InstanceState{
			ID: volumeAttachmentID(deviceName, volumeID, instanceID),
			Attributes: map[string]string{
				"id":          volumeAttachmentID(deviceName, volumeID, instanceID),
				"device_name": deviceName,
				"instance_id": instanceID,
				"volume_id":   volumeID,
			},
			Meta:    make(map[string]interface{}),
			Tainted: false,
		},
		Deposed: []*terraform.InstanceState{},
	}
}

// Calculate the "vai-xxx" value
// From https://github.com/foxsy/tfvolattid/blob/master/tfvolattid.go
func volumeAttachmentID(name, volumeID, instanceID string) string {
	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("%s-", name))
	buf.WriteString(fmt.Sprintf("%s-", instanceID))
	buf.WriteString(fmt.Sprintf("%s-", volumeID))

	return fmt.Sprintf("vai-%d", hashcode.String(buf.String()))
}
