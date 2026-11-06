package ec2query

func (h *handler) serve() {
	switch action {
	case resource.ActionRunInstances.String():
		h.runInstances()
	case "DescribeVpcs":
		h.describeVpcs()
	default:
		writeErr(resource.Unimplemented(action))
	}
}
