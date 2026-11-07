package resource

type Action int

func (a Action) String() string {
	switch a {
	case ActionRunInstances:
		return "RunInstances"
	case actionNone:
		return ""
	default:
		return ""
	}
}

func (o Other) String() string { return "ignored" }
