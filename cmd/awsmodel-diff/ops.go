package main

import (
	"fmt"
	"slices"

	"github.com/popododo0720/vws-conformance/internal/bind"
)

// implemented is the Impl value of a case whose operation VWS serves; "gap" cases are operations
// VWS does not serve yet.
const implemented = "yes"

// modelService maps a catalog service to the botocore model that defines its operations. The
// auth cases sign calls to operations of other services (sts GetCallerIdentity, s3 ListBuckets)
// that those services' own cases already list, so auth maps to no model.
var modelService = map[string]string{
	"ec2": "ec2", "vpc": "ec2", "ebs": "ec2",
	"s3": "s3", "iam": "iam", "sts": "sts",
	"auth": "",
}

// UnmappedServiceError is a catalog service with no entry in modelService. A new service package
// fails the run until it is mapped, so its operations are never silently left out of the diff.
type UnmappedServiceError struct {
	Service string
}

func (e *UnmappedServiceError) Error() string {
	return fmt.Sprintf("awsmodel: catalog service %q has no botocore model in modelService", e.Service)
}

// implementedOps returns, per botocore service, the sorted operations that the case catalog
// marks as implemented. The catalog (engine.Groups) is the only operation list.
func implementedOps(groups []bind.Group) (map[string][]string, error) {
	ops := map[string][]string{}
	for _, g := range groups {
		for _, c := range g.Cases {
			svc, ok := modelService[c.Service]
			if !ok {
				return nil, &UnmappedServiceError{Service: c.Service}
			}
			if svc == "" || c.Impl != implemented || slices.Contains(ops[svc], c.Op) {
				continue
			}
			ops[svc] = append(ops[svc], c.Op)
		}
	}
	for _, list := range ops {
		slices.Sort(list)
	}
	return ops, nil
}
