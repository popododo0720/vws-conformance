package iamquery

func actionTable() map[sigv4.Service]map[string]spec {
	return map[sigv4.Service]map[string]spec{sigv4.ServiceIAM: iamActions(), sigv4.ServiceSTS: stsActions()}
}

func iamActions() map[string]spec {
	m := map[string]spec{
		"CreateUser": admin(),
	}
	for kind, e := range map[iam.Entity]struct {
		word, key string
		arn       func()
	}{
		iam.EntityUser:  {"User", "UserName", userARN},
		iam.EntityGroup: {"Group", "GroupName", groupARN},
	} {
		kind, e := kind, e
		m["Attach"+e.word+"Policy"] = admin()
		m[("Get" + e.word)] = admin()
	}
	return m
}

func stsActions() map[string]spec {
	return map[string]spec{"GetCallerIdentity": {}}
}
