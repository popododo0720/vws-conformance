package inventory

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

func FromCheckout(root string) ([]Op, error) {
	var out []Op
	ec2, err := os.ReadFile(filepath.Join(root, "internal/surface/ec2query/ec2query.go"))
	if err != nil {
		return nil, err
	}
	seen := map[string]bool{}
	add := func(service, name, impl string) {
		key := service + " " + name
		if seen[key] {
			return
		}
		seen[key] = true
		out = append(out, Op{Service: service, Name: name, Impl: impl})
	}
	action := regexp.MustCompile(`case resource\.Action([A-Za-z]+)\.String\(\):`)
	quoted := regexp.MustCompile(`case "([A-Za-z]+)":`)
	for _, m := range action.FindAllStringSubmatch(string(ec2), -1) {
		svc, err := ec2Service(m[1])
		if err != nil {
			return nil, err
		}
		add(svc, m[1], "yes")
	}
	for _, m := range quoted.FindAllStringSubmatch(string(ec2), -1) {
		if m[1] == "DescribeInstanceAttribute" || strings.HasPrefix(m[1], "Describe") || m[1] == "ImportKeyPair" {
			svc, err := ec2Service(m[1])
			if err != nil {
				return nil, err
			}
			add(svc, m[1], "yes")
		}
	}
	s3, err := os.ReadFile(filepath.Join(root, "internal/surface/s3rest/s3rest.go"))
	if err != nil {
		return nil, err
	}
	routes := map[string][]string{
		`mux.HandleFunc("GET /{$}", h.listBuckets)`:                   {"ListBuckets"},
		`mux.HandleFunc("GET /{bucket}", h.listObjects)`:              {"ListObjects", "ListObjectsV2"},
		`mux.HandleFunc("HEAD /{bucket}", h.headBucket)`:              {"HeadBucket"},
		`mux.HandleFunc("PUT /{bucket}", h.createBucket)`:             {"CreateBucket"},
		`mux.HandleFunc("DELETE /{bucket}", h.deleteBucket)`:          {"DeleteBucket"},
		`mux.HandleFunc("PUT /{bucket}/{key...}", h.putObject)`:       {"PutObject"},
		`mux.HandleFunc("GET /{bucket}/{key...}", h.getObject)`:       {"GetObject"},
		`mux.HandleFunc("HEAD /{bucket}/{key...}", h.headObject)`:     {"HeadObject"},
		`mux.HandleFunc("DELETE /{bucket}/{key...}", h.deleteObject)`: {"DeleteObject"},
	}
	text := string(s3)
	for line, ops := range routes {
		if !strings.Contains(text, line) {
			return nil, fmt.Errorf("s3 route missing: %s", line)
		}
		for _, op := range ops {
			add("s3", op, "yes")
		}
	}
	iam, err := os.ReadFile(filepath.Join(root, "internal/surface/iamquery/actions.go"))
	if err != nil {
		return nil, err
	}
	body := string(iam)
	start := strings.Index(body, "func iamActions()")
	sts := strings.Index(body, "func stsActions()")
	if start < 0 || sts < 0 || sts < start {
		return nil, fmt.Errorf("iam action maps were not found")
	}
	key := regexp.MustCompile(`"([A-Za-z]+)"\s*:`)
	for _, m := range key.FindAllStringSubmatch(body[start:sts], -1) {
		add("iam", m[1], "yes")
	}
	if strings.Contains(body[start:sts], `e.word+"Policy"`) {
		for _, word := range []string{"User", "Group", "Role"} {
			add("iam", "Attach"+word+"Policy", "yes")
			add("iam", "Detach"+word+"Policy", "yes")
			add("iam", "ListAttached"+word+"Policies", "yes")
			add("iam", "Put"+word+"Policy", "yes")
			add("iam", "Get"+word+"Policy", "yes")
			add("iam", "Delete"+word+"Policy", "yes")
			add("iam", "List"+word+"Policies", "yes")
		}
	}
	stsBody := body[sts:]
	for _, op := range []string{"GetCallerIdentity", "AssumeRole"} {
		if !strings.Contains(stsBody, `"`+op+`"`) {
			return nil, fmt.Errorf("sts action %s missing", op)
		}
		add("sts", op, "yes")
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Service != out[j].Service {
			return out[i].Service < out[j].Service
		}
		return out[i].Name < out[j].Name
	})
	return out, nil
}

func ec2Service(op string) (string, error) {
	switch op {
	case "CreateVpc", "DeleteVpc", "DescribeVpcs", "CreateSubnet", "DeleteSubnet", "DescribeSubnets",
		"CreateSecurityGroup", "DeleteSecurityGroup", "AuthorizeSecurityGroupIngress", "DescribeSecurityGroups", "ImportKeyPair":
		if op == "ImportKeyPair" {
			return "ec2", nil
		}
		if strings.Contains(op, "SecurityGroup") {
			return "vpc", nil
		}
		if strings.Contains(op, "Subnet") {
			return "vpc", nil
		}
		return "vpc", nil
	case "CreateVolume", "DeleteVolume", "DescribeVolumes", "AttachVolume", "ModifyVolume",
		"CreateSnapshot", "DeleteSnapshot", "DescribeSnapshots":
		return "ebs", nil
	case "RunInstances", "StopInstances", "StartInstances", "TerminateInstances", "CreateTags",
		"CreateKeyPair", "DeleteKeyPair", "DescribeKeyPairs", "AllocateAddress", "ReleaseAddress",
		"AssociateAddress", "DisassociateAddress", "DescribeAddresses", "DescribeInstances",
		"DescribeImages", "DescribeInstanceAttribute":
		return "ec2", nil
	default:
		return "", fmt.Errorf("ec2 action %s has no service mapping", op)
	}
}

func SameImpl(file, live []Op) error {
	want := map[string]bool{}
	got := map[string]bool{}
	for _, op := range file {
		if op.Impl == "yes" {
			want[op.Service+" "+op.Name] = true
		}
	}
	for _, op := range live {
		got[op.Service+" "+op.Name] = true
	}
	var diff []string
	for key := range want {
		if !got[key] {
			diff = append(diff, "inventory has "+key+" but the checkout does not")
		}
	}
	for key := range got {
		if !want[key] {
			diff = append(diff, "checkout has "+key+" but the inventory does not")
		}
	}
	if len(diff) == 0 {
		return nil
	}
	sort.Strings(diff)
	return fmt.Errorf("%s", strings.Join(diff, "; "))
}
