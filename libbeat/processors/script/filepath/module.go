package filepath

import (
	"path/filepath"

	"github.com/yuin/gopher-lua"
)

const ModuleName = "filepath"

func Loader(L *lua.LState) int {
	// register functions to the table
	mod := L.SetFuncs(L.NewTable(), exports)
	// register other stuff
	L.SetField(mod, "name", lua.LString(ModuleName))

	// returns the module
	L.Push(mod)
	return 1
}

var exports = map[string]lua.LGFunction{
	"base": base,
	"dir":  dir,
	"ext":  ext,
}

func dir(L *lua.LState) int {
	if L.GetTop() != 1 {
		L.ArgError(1, "path arg expected")
		return 0
	}

	path := L.CheckString(1)
	rtn := filepath.Dir(path)
	L.Push(lua.LString(rtn))
	return 1
}

func base(L *lua.LState) int {
	if L.GetTop() != 1 {
		L.ArgError(1, "path arg expected")
		return 0
	}

	path := L.CheckString(1)
	rtn := filepath.Base(path)
	L.Push(lua.LString(rtn))
	return 1
}

func ext(L *lua.LState) int {
	if L.GetTop() != 1 {
		L.ArgError(1, "path arg expected")
		return 0
	}

	path := L.CheckString(1)
	rtn := filepath.Ext(path)
	L.Push(lua.LString(rtn))
	return 1
}
