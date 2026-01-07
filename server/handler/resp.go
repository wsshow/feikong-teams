package handler

type Response struct {
	Code int    `json:"code"`
	Desc string `json:"message"`
	Data any    `json:"data"`
}

var resp Response

func (r Response) Success(data any) Response {
	return Response{
		Code: 0,
		Desc: "success",
		Data: data,
	}
}

func (r Response) Failure() Response {
	return Response{
		Code: 1,
		Desc: "failure",
		Data: nil,
	}
}

func (r Response) WithDesc(desc string) Response {
	r.Desc = desc
	return r
}
