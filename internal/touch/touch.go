package touch

type Group struct {
	Service string
	Name    string
}

type Rule struct {
	Service  string
	Group    string
	Prefixes []string
	Big      bool
}

var Rules = []Rule{
	{Service: "s3", Group: "buckets", Prefixes: []string{"s3/buckets"}},
	{Service: "s3", Group: "listings", Prefixes: []string{"s3/listings"}},
	{Service: "ec2", Group: "instances", Prefixes: []string{"ec2/instances"}},
	{Service: "ec2", Group: "images", Prefixes: []string{"ec2/images"}},
	{Service: "ec2", Group: "keypairs", Prefixes: []string{"ec2/keypairs"}},
	{Service: "ec2", Group: "addresses", Prefixes: []string{"ec2/addresses"}},
	{Service: "ec2", Group: "gaps", Prefixes: []string{"ec2/gaps"}},
	{Service: "vpc", Group: "vpcs", Prefixes: []string{"vpc/vpcs"}},
	{Service: "vpc", Group: "subnets", Prefixes: []string{"vpc/subnets"}},
	{Service: "vpc", Group: "securitygroups", Prefixes: []string{"vpc/securitygroups"}},
	{Service: "vpc", Group: "gaps", Prefixes: []string{"vpc/gaps"}},
	{Service: "ebs", Group: "volumes", Prefixes: []string{"ebs/volumes"}},
	{Service: "ebs", Group: "snapshots", Prefixes: []string{"ebs/snapshots"}},
	{Service: "ebs", Group: "gaps", Prefixes: []string{"ebs/gaps"}},
	{Service: "iam", Group: "users", Prefixes: []string{"iam/"}},
	{Service: "sts", Group: "caller", Prefixes: []string{"sts/"}},
	{Service: "auth", Group: "sigv4", Prefixes: []string{"auth/"}},
	{Big: true, Prefixes: []string{
		"internal/", "schema/", "cmd/", "go.mod", "go.sum", ".github/workflows/", "scripts/",
	}},
}

type Selection struct {
	Full   bool
	Groups []Group
}

func Select(files []string) Selection {
	seen := map[Group]bool{}
	var sel Selection
	for _, f := range files {
		for _, rule := range Rules {
			if !match(f, rule.Prefixes) {
				continue
			}
			if rule.Big {
				sel.Full = true
				continue
			}
			g := Group{Service: rule.Service, Name: rule.Group}
			if !seen[g] {
				seen[g] = true
				sel.Groups = append(sel.Groups, g)
			}
		}
	}
	if sel.Full {
		sel.Groups = nil
	}
	return sel
}

func match(file string, prefixes []string) bool {
	for _, p := range prefixes {
		if len(file) >= len(p) && file[:len(p)] == p {
			return true
		}
	}
	return false
}
