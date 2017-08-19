// Copyright 2016 The Gosl Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package ode

import "github.com/cpmech/gosl/la"

// Solve solves ODE problem using standard parameters
//
//  INPUT:
//   method    -- the method
//   fcn       -- function d{y}/dx := {f}(h=dx, x, {y})
//   jac       -- Jacobian function d{f}/d{y} := [J](h=dx, x, {y}) [may be nil]
//   y         -- current {y} @ x=0
//   xf        -- final x
//   dx        -- stepsize. [may be used for dense output]
//   atol      -- absolute tolerance; use 0 for default [default = 1e-4] (for fixedStp=false)
//   rtol      -- relative tolerance; use 0 for default [default = 1e-4] (for fixedStp=false)
//   numJac    -- use numerical Jacobian if if jac is non nil
//   fixedStep -- fixed steps
//   saveStep  -- save steps
//   saveDense -- save many steps (dense output) [using dx]
//
//  OUTPUT:
//   yf   -- final {y}
//   stat -- statistics
//   out  -- output with all steps results with save==true
//
func Solve(method string, fcn Func, jac JacF, y la.Vector, xf, dx, atol, rtol float64,
	numJac, fixedStep, saveStep, saveDense bool) (yf la.Vector, stat *Stat, out *Output, err error) {

	// current y vector
	ndim := len(y)
	yf = la.NewVector(ndim)
	yf.Apply(1, y)

	// configuration
	conf, err := NewConfig(method, "", nil)
	if err != nil {
		return
	}
	if atol > 0 && rtol > 0 {
		conf.SetTol(atol, rtol)
	}
	if fixedStep {
		conf.SetFixedH(dx, xf)
	}
	if saveStep {
		conf.SetStepOut(true, nil)
	}
	if saveDense {
		conf.SetDenseOut(true, dx, xf, nil)
	}

	// output handler
	out = NewOutput(ndim, conf)

	// allocate solver
	J := jac
	if numJac {
		J = nil
	}
	sol, err := NewSolver(ndim, conf, out, fcn, J, nil)
	if err != nil {
		return
	}
	defer sol.Free()

	// solve ODE
	err = sol.Solve(yf, 0.0, xf)

	// set stat variable
	stat = sol.Stat
	return
}
