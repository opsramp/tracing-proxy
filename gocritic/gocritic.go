//go:build ruleguard
// +build ruleguard

package gorules

import (
	"github.com/quasilyte/go-ruleguard/dsl"
)

func ifacePtr(m dsl.Matcher) {
	m.Match(`*$x`).
		Where(m["x"].Type.Underlying().Is(`interface{ $*_ }`)).
		Report(`don't use pointers to an interface`)
}

func newMutex(m dsl.Matcher) {
	m.Match(`$mu := new(sync.Mutex); $mu.Lock()`).
		Report(`use zero mutex value instead, 'var $mu sync.Mutex'`).
		At(m["mu"])
}

func channelSize(m dsl.Matcher) {
	m.Match(`make(chan $_, $size)`).
		Where(m["size"].Value.Int() != 0 && m["size"].Value.Int() != 1).
		Report(`channels should have a size of one or be unbuffered`)
}

func uncheckedTypeAssert(m dsl.Matcher) {
	m.Match(
		`$_ := $_.($_)`,
		`$_ = $_.($_)`,
		`$_($*_, $_.($_), $*_)`,
		`$_{$*_, $_.($_), $*_}`,
		`$_{$*_, $_: $_.($_), $*_}`).
		Report(`avoid unchecked type assertions as they can panic`)
}

// For cases where there's an unnecessary else clause
//
//	 var x int
//		if unlikely(x) {
//		  x = 123
//		} else {
//		  x = 444
//		}
//
// re-write as
//
//	 x := 444
//		if notLikely(x) {
//			 x = 123
//		}
func unnecessaryElse(m dsl.Matcher) {
	m.Match(`var $v $_; if $cond { $v = $x } else { $v = $y }`).
		Where(m["y"].Pure).
		Report(`rewrite as '$v := $y; if $cond { $v = $x }'`)
	//Suggest(`$v := $y; if $cond { $v = $x }`)
}

func localVarDecl(m dsl.Matcher) {
	m.Match(`func $name($*vars) $ret{
		$*body 
		var $v = $y
		$*trailing 
	  }`).At(m["v"]).
		Report(`var used for assignment; use '$v := $y'`)
}

// errorWrapping checks Go Uber error wrapping rules.
//
// For example, instead of this:
//
//	fmt.Errorf("failed to do something: %w", err)
//
// you should prefer this (without "failed to"):
//
//	fmt.Errorf("do something: %w", err)
//
// according to https://github.com/uber-go/guide/blob/master/style.md#error-wrapping.
func errorWrapping(m dsl.Matcher) {
	m.Match(`$pkg.Errorf($msg, $*_)`).Where(
		m["msg"].Text.Matches(`^"failed to .*: %[wvs]"$`) &&
			m["pkg"].Text.Matches("fmt|errors|xerrors"),
	).Report(`"failed to" message part is redundant in error wrapping`)

	m.Match(`$pkg.Wrap($_, $msg)`).Where(
		m["msg"].Text.Matches(`^"failed to .*"$`) &&
			m["pkg"].Text.Matches("errors|xerrors"),
	).Report(`"failed to" message part is redundant in error wrapping`)

	m.Match("$pkg.Wrapf($_, $msg, $*_)").Where(
		m["msg"].Text.Matches(`^"failed to .*: %[wvs]"$`) &&
			m["pkg"].Text.Matches("errors|xerrors"),
	).Report(`"failed to" message part is redundant in error wrapping`)
}

// for cases where a mutex is embedded directly in a struct
//
//	type MyThing struct {
//	   sync.Mutex   // this is bad, enables public control of the lock
//	   ...
//	}
//
// same for embedding a sync.RWMutex
func embeddedMutex(m dsl.Matcher) {
	isExported := func(v dsl.Var) bool {
		return v.Text.Matches(`^\p{Lu}`)
	}

	m.Match(`type $name struct { $*_; sync.Mutex; $*_ }`).
		Where(isExported(m["name"])).
		Report(`do not embed sync.Mutex`)

	m.Match(`type $name struct { $*_; sync.RWMutex; $*_ }`).
		Where(isExported(m["name"])).
		Report(`do not embed sync.RWMutex`)
}

// for a zero sized slice, use var instead to create a typed nil.
func makeSliceNoSize(m dsl.Matcher) {
	m.Match(`make([]$s, 0)`).
		Report(`make slice without size; use 'var []$s'`)
}

// Use an option interface rather than closures for parameters to
// constructors.
func optionClosure(m dsl.Matcher) {
	m.Match(`
	func $name($*vars) $ret {
		return func($*ovars) $r { 
			$*body 
		}
	}`).
		Where(m["name"].Text.Matches("^With*")).
		Report(`use Option interface not closures`)
}

// returning an empty slice
func nilSlice(m dsl.Matcher) {
	m.Match(`return []$t{}`).
		Report(`returning empty slice; use 'return nil'`)
}

// declaration of empty slice
func emptySliceDeclaration(m dsl.Matcher) {
	m.Match(`$v := []$t{}`).
		Report(`empty slice declaration; use 'var $v []$t'`)
}

func errScope(m dsl.Matcher) {
	m.Match(`if $*vars = $f; $cond { $*_ }`).
		Where(m["vars"].Contains("err")).
		Report(`reduce scope; use 'if $vars := $f'`)
}

func nakedBool(m dsl.Matcher) {
	m.Match(`$f($*vars)`).
		Where(m["vars"].Contains("true") && (!m["vars"].Contains("func($*_) $*_"))).
		Report(`do not use naked booleans in call parameters`)

	m.Match(`$f($*vars)`).
		Where(m["vars"].Contains("false") && (!m["vars"].Contains("func($*_) $*_"))).
		Report(`do not use naked booleans in call parameters`)
}

func emptyStringTest(m dsl.Matcher) {
	m.Match(`len($s) == 0`).
		Where(m["s"].Type.Is("string")).
		Report(`maybe use $s == "" instead?`)

	m.Match(`len($s) != 0`).
		Where(m["s"].Type.Is("string")).
		Report(`maybe use $s != "" instead?`)
}

func varZeroValueStruct(m dsl.Matcher) {
	m.Match(`$v := $s{}`).
		Where(!m["s"].Type.Underlying().Is(`map[$k]$v`)).
		Report("use var for zero value struct; var $v $s{}")
}

func noImmediateUnlockDefer(m dsl.Matcher) {
	m.Match(`
	func $name($*vars) $ret {
		$*_
		$m.Lock()
		$*trailing
	}`).
		Where(!m["trailing"].Text.Matches(`^defer (.*)\.Unlock()`)).At(m["m"]).
		Report(`provide an immediate unlock; 'defer $m.Unlock()'`)

	m.Match(`
	func $name($*vars) $ret {
		$*_
		$m.RLock()
		$*trailing
	}`).
		Where(!m["trailing"].Text.Matches(`^defer (.*)\.RUnlock()`)).At(m["m"]).
		Report(`provide an immediate unlock; 'defer $m.RUnlock()'`)

	// receivers
	m.Match(`
		func ($*_) $name($*vars) $ret {
			$*_
			$m.Lock()
			$*trailing
		}`).
		Where(!m["trailing"].Text.Matches(`^defer (.*)\.Unlock()`)).At(m["m"]).
		Report(`provide an immediate unlock; 'defer $m.Unlock()'`)

	m.Match(`
		func ($*_) $name($*vars) $ret {
			$*_
			$m.RLock()
			$*trailing
		}`).
		Where(!m["trailing"].Text.Matches(`^defer (.*)\.RUnlock()`)).At(m["m"]).
		Report(`provide an immediate unlock; 'defer $m.RUnlock()'`)
}
