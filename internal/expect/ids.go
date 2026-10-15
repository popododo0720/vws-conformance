package expect

const (
	InstanceID = `^i-[0-9a-f]{8,17}$`
	VolumeID   = `^vol-[0-9a-f]{8,17}$`
	VpcID      = `^vpc-[0-9a-f]{8,17}$`
	SubnetID   = `^subnet-[0-9a-f]{8,17}$`
	GroupID    = `^sg-[0-9a-f]{8,17}$`
	SnapID     = `^snap-[0-9a-f]{8,17}$`
	AllocID    = `^eipalloc-[0-9a-f]{8,17}$`
	AssocID    = `^eipassoc-[0-9a-f]{8,17}$`
	KeyID      = `^key-[0-9a-f]{8,17}$`
	ImageID    = `^ami-[0-9a-f]{8,17}$`
	ARN        = `^arn:aws:iam::[0-9]{12}:`
)
