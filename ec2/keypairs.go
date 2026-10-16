package ec2

import (
	"context"
	"crypto/ed25519"
	"crypto/rand"
	"encoding/base64"
	"encoding/binary"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsec2 "github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"github.com/popododo0720/vws-conformance/internal/bind"
	"github.com/popododo0720/vws-conformance/internal/cases"
	"github.com/popododo0720/vws-conformance/internal/harness"
)

func KeyPairs() bind.Group {
	return bind.Group{
		Service: "ec2", Name: "keypairs", Run: runKeys,
		Cases: []cases.Case{
			{
				ID: "EC2-KP-001", Service: "ec2", Group: "keypairs", Op: "CreateKeyPair", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("CreateKeyPair"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "EC2-KP-002", Service: "ec2", Group: "keypairs", Op: "DescribeKeyPairs", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DescribeKeyPairs"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "EC2-KP-003", Service: "ec2", Group: "keypairs", Op: "DeleteKeyPair", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("DeleteKeyPair"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "EC2-KP-004", Service: "ec2", Group: "keypairs", Op: "ImportKeyPair", Kind: cases.KindHappy,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2("ImportKeyPair"), Expect: cases.Expect{HTTP: 200}, Impl: "yes",
			},
			{
				ID: "EC2-KP-101", Service: "ec2", Group: "keypairs", Op: "CreateKeyPair", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidKeyPair.Duplicate",
				}, Impl: "yes",
			},
			{
				ID: "EC2-KP-102", Service: "ec2", Group: "keypairs", Op: "DeleteKeyPair", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidKeyPair.NotFound",
				}, Impl: "yes",
			},
			{
				ID: "EC2-KP-103", Service: "ec2", Group: "keypairs", Op: "DescribeKeyPairs", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{
					HTTP: 400,
					Code: "InvalidKeyPair.NotFound",
				}, Impl: "yes",
			},
			{
				ID: "EC2-KP-104", Service: "ec2", Group: "keypairs", Op: "ImportKeyPair", Kind: cases.KindError,
				Suite: cases.SuiteSDK, DocURL: cases.DocEC2Errors, Expect: cases.Expect{HTTP: 400, Code: "InvalidKey.Format"},
				Impl: "yes",
			},
		},
	}
}

func runKeys(ctx context.Context, rt *harness.Runtime, c cases.Case) (cases.Outcome, error) {
	api, err := client(rt)
	if err != nil {
		return cases.Outcome{}, err
	}
	name := rt.Name(strings.ToLower(c.ID))
	switch c.ID {
	case "EC2-KP-001":
		var created bool
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.CreateKeyPair(ctx, &awsec2.CreateKeyPairInput{
				KeyName: aws.String(name), KeyType: types.KeyTypeRsa,
				KeyFormat: types.KeyFormatPem,
			})
			created = err == nil
			return wrapErr("runKeys", err)
		})
		if created {
			deleteKey(rt, name)
		}
		return o, wrapErr("runKeys", err)
	case "EC2-KP-002":
		if _, err := api.CreateKeyPair(ctx, &awsec2.CreateKeyPairInput{KeyName: aws.String(name)}); err != nil {
			return cases.Outcome{}, fmt.Errorf("create: %w", err)
		}
		deleteKey(rt, name)
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeKeyPairs(ctx, &awsec2.DescribeKeyPairsInput{KeyNames: []string{name}})
				return wrapErr("runKeys", err)
			})
			return callOut, wrapErr("runKeys", callErr)
		}
	case "EC2-KP-003":
		if _, err := api.CreateKeyPair(ctx, &awsec2.CreateKeyPairInput{KeyName: aws.String(name)}); err != nil {
			return cases.Outcome{}, fmt.Errorf("create: %w", err)
		}
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				err := rt.EC2DeleteKeyPair(ctx, name)
				return wrapErr("runKeys", err)
			})
			return callOut, wrapErr("runKeys", callErr)
		}
	case "EC2-KP-004":
		pub, err := ed25519Public()
		if err != nil {
			return cases.Outcome{}, err
		}
		var created bool
		o, err := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.ImportKeyPair(ctx, &awsec2.ImportKeyPairInput{KeyName: aws.String(name), PublicKeyMaterial: pub})
			created = err == nil
			return wrapErr("runKeys", err)
		})
		if created {
			deleteKey(rt, name)
		}
		return o, wrapErr("runKeys", err)
	case "EC2-KP-101":
		if _, err := api.CreateKeyPair(ctx, &awsec2.CreateKeyPairInput{KeyName: aws.String(name)}); err != nil {
			return cases.Outcome{}, fmt.Errorf("create: %w", err)
		}
		deleteKey(rt, name)
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.CreateKeyPair(ctx, &awsec2.CreateKeyPairInput{KeyName: aws.String(name)})
				return wrapErr("runKeys", err)
			})
			return callOut, wrapErr("runKeys", callErr)
		}
	case "EC2-KP-102":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				err := rt.EC2DeleteKeyPair(ctx, rt.Name("missing"))
				return wrapErr("runKeys", err)
			})
			return callOut, wrapErr("runKeys", callErr)
		}
	case "EC2-KP-103":
		{
			callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
				_, err := api.DescribeKeyPairs(ctx, &awsec2.DescribeKeyPairsInput{KeyNames: []string{rt.Name("missing")}})
				return wrapErr("runKeys", err)
			})
			return callOut, wrapErr("runKeys", callErr)
		}
	case "EC2-KP-104":
		var created bool
		callOut, callErr := rt.Call(ctx, func(ctx context.Context) error {
			_, err := api.ImportKeyPair(ctx, &awsec2.ImportKeyPairInput{
				KeyName:           aws.String(name),
				PublicKeyMaterial: []byte("not-a-key"),
			})
			created = err == nil
			return wrapErr("runKeys", err)
		})
		if created {
			deleteKey(rt, name)
		}
		if harness.SetupReady(callOut, callErr) && !created {
			return callOut, fmt.Errorf("import: success did not identify a key")
		}
		return callOut, wrapErr("runKeys", callErr)
	default:
		return cases.Outcome{}, fmt.Errorf("no runner for %s", c.ID)
	}
}

func deleteKey(rt *harness.Runtime, name string) {
	rt.Cleanup(func(ctx context.Context) error {
		return rt.DeleteOwnedKey(ctx, name)
	})
}

func ed25519Public() ([]byte, error) {
	pub, _, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("ed25519: %w", err)
	}
	var buf []byte
	buf = appendSSH(buf, []byte("ssh-ed25519"))
	buf = appendSSH(buf, pub)
	return []byte("ssh-ed25519 " + base64.StdEncoding.EncodeToString(buf) + " conformance"), nil
}

func appendSSH(dst, s []byte) []byte {
	var n [4]byte
	binary.BigEndian.PutUint32(n[:], uint32(len(s)))
	dst = append(dst, n[:]...)
	return append(dst, s...)
}
