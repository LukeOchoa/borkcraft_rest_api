package errorc

type ErrorComplex struct {
	Code int 
	Msg  string
	Err  error
}

func (errc ErrorComplex) Error() string {
	if errc.Err != nil {
		return errc.Err.Error()
	}

	return errc.Msg
}

func New(code int, msg string, err error) ErrorComplex {
	return ErrorComplex{
		Code: code,
		Msg: msg,
		Err: err,
	}
}


func Nil() ErrorComplex {
	return ErrorComplex{
		Code: 0,
		Msg: "",
		Err: nil,
	}
}
