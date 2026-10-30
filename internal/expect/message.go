package expect

// documentedMessage is the stable phrase from the AWS error reference.
// Quoted identifiers, parameter names, and request-specific values are `.+`.
//
// EC2: https://docs.aws.amazon.com/AWSEC2/latest/APIReference/errors-overview.html
// S3: https://docs.aws.amazon.com/AmazonS3/latest/API/ErrorResponses.html
// IAM: https://docs.aws.amazon.com/IAM/latest/APIReference/CommonErrors.html
// IAM access denied: https://docs.aws.amazon.com/IAM/latest/UserGuide/troubleshoot_access-denied.html
var documentedMessage = map[string]string{
	"InvalidVolume.NotFound":        `(?i)volume '.+' does not exist`,
	"InvalidSnapshot.NotFound":      `(?i)snapshot '.+' does not exist`,
	"InvalidInstanceID.NotFound":    `(?i)instance id '.+' does not exist`,
	"InvalidInstanceID.Malformed":   `(?i)instance id '.+' is malformed`,
	"InvalidAMIID.NotFound":         `(?i)image id '.+' does not exist`,
	"InvalidAllocationID.NotFound":  `(?i)allocation id '.+' does not exist`,
	"InvalidAssociationID.NotFound": `(?i)association id '.+' does not exist`,
	"InvalidKeyPair.NotFound":       `(?i)key ?pair '.+' does not exist`,
	"InvalidKeyPair.Duplicate":      `(?i)key ?pair '.+' already exists`,
	"InvalidKey.Format":             `(?i)key is not in valid openssh public key format`,
	"InvalidGroup.NotFound":         `(?i)security group '.+' does not exist`,
	"InvalidGroup.Duplicate":        `(?i)security group '.+' already exists`,
	"InvalidPermission.Duplicate":   `(?i)specified rule.+already exists`,
	"InvalidVpcID.NotFound":         `(?i)vpc id '.+' does not exist`,
	"InvalidSubnetID.NotFound":      `(?i)subnet id '.+' does not exist`,
	"InvalidVpc.Range":              `(?i)cidr '.+' is invalid`,
	"InvalidSubnet.Range":           `(?i)cidr '.+' is invalid`,
	"VolumeInUse":                   `(?i)volume .+ is currently attached`,
	"DependencyViolation":           `(?i)has dependenc`,
	"MissingParameter":              `(?i)request must contain the parameter .+`,
	"IdempotentParameterMismatch":   `(?i)client token .+ different parameters`,
	"InvalidParameterValue":         `(?i)(value .+ for parameter .+ is invalid|parameter .+ is invalid)`,
	"IncorrectState":                `(?i)not in the .+ state`,
	"NoSuchBucket":                  `(?i)specified bucket does not exist`,
	"NoSuchKey":                     `(?i)specified key does not exist`,
	"BucketNotEmpty":                `(?i)bucket you tried to delete is not empty`,
	"BucketAlreadyOwnedByYou":       `(?i)already own it`,
	"InvalidBucketName":             `(?i)specified bucket is not valid`,
	"MalformedXML":                  `(?i)xml you provided was not well-formed`,
	"BadDigest":                     `(?i)content-md5 you specified did not match`,
	"InvalidRange":                  `(?i)requested range (is not satisfiable|cannot be satisfied)`,
	"PreconditionFailed":            `(?i)at least one of the preconditions you specified did not hold`,
	"AccessDenied":                  `(?i)(access denied\.?|not authorized to perform)`,
	"SignatureDoesNotMatch":         `(?i)request signature we calculated does not match the signature you provided`,
	"RequestTimeTooSkewed":          `(?i)difference between the request time and the current time is too large`,
	"AuthorizationHeaderMalformed":  `(?i)authorization header is malformed`,
	"InvalidAccessKeyId":            `(?i)aws access key id you provided does not exist in our records`,
	"ExpiredToken":                  `(?i)provided token has expired`,
	"UnauthorizedOperation":         `(?i)you are not authorized to perform this operation`,
	"EntityAlreadyExists":           `(?i)already exists`,
	"NoSuchEntity":                  `(?i)(cannot be found|does not exist)`,
	"DeleteConflict":                `(?i)cannot be deleted`,
	"ValidationError":               `(?i)validation error`,
	"LimitExceeded":                 `(?i)cannot exceed quota`,
	"MalformedPolicyDocument":       `(?i)syntax errors in policy`,
	"InvalidClientTokenId":          `(?i)security token included in the request is invalid`,
	"InvalidIdentityToken":          `(?i)identity token`,
}

func messagePattern(code string) string {
	return documentedMessage[code]
}
