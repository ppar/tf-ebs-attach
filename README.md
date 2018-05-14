
# tf-ebs-attach

This tool lets you "import" an AWS EBS volume attachment into your Terraform 
state file. While Terraform lets you import AWS instances and EBS volumes, it 
does not support importing the synthetic "aws_volume_attachment" resource that 
has no identifiable counterpart in AWS.  

## Usage
```
Usage:
  tf-ebs-attach import [-i f] [-o f] <inst-name> <vol-name> <att-name> <dev>  
  tf-ebs-attach show <inst-id> <vol-name> <vol-id> <att-name> <dev>
  tf-ebs-attach --help
  
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

```

## Acknowledgements

https://github.com/foxsy/tfvolattid/

## Compilation

- Install Go (1.9+) and glide
- Run `make`
