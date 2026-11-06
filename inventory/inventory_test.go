package inventory

import "testing"

func sample() Inventory {
	return Inventory{Commit: fixtureCommit, Ops: map[Op]bool{
		{EC2, "RunInstances"}: true, {EC2, "DescribeVpcs"}: true, {S3, "ListBuckets"}: true,
	}}
}

func TestEncodeIsCanonical(t *testing.T) {
	want := `{
  "schema_version": "1",
  "console_api": {
    "repo": "popododo0720/vws-console-api",
    "commit": "` + fixtureCommit + `"
  },
  "operations": {
    "ec2": [
      "DescribeVpcs",
      "RunInstances"
    ],
    "s3": [
      "ListBuckets"
    ]
  }
}
`
	got, err := Encode(sample())
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != want {
		t.Errorf("encoding\n got %s\nwant %s", got, want)
	}
}

func TestEncodeRejectsBadInventory(t *testing.T) {
	for name, inv := range map[string]Inventory{
		"bad commit":  {Commit: "x", Ops: sample().Ops},
		"unknown api": {Commit: fixtureCommit, Ops: map[Op]bool{{"kms", "Decrypt"}: true}},
		"empty api":   {Commit: fixtureCommit, Ops: map[Op]bool{{EC2, "RunInstances"}: true}},
		"bad name": {Commit: fixtureCommit, Ops: map[Op]bool{
			{EC2, "Run Instances"}: true, {S3, "ListBuckets"}: true,
		}},
	} {
		if _, err := Encode(inv); err == nil {
			t.Errorf("%s: want an error", name)
		}
	}
}
