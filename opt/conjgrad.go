// Copyright 2016 The Gosl Authors. All rights reserved.
// Use of this source code is governed by a BSD-style
// license that can be found in the LICENSE file.

package opt

import (
	"math"

	"github.com/cpmech/gosl/chk"
	"github.com/cpmech/gosl/fun"
	"github.com/cpmech/gosl/la"
	"github.com/cpmech/gosl/num"
	"github.com/cpmech/gosl/utl"
)

// ConjGrad implements the multidimensional minimization by the Fletcher-Reeves-Polak-Ribiere method.
//
//   REFERENCES:
//   [1] Press WH, Teukolsky SA, Vetterling WT, Fnannery BP (2007) Numerical Recipes:
//       The Art of Scientific Computing. Third Edition. Cambridge University Press. 1235p.
//
type ConjGrad struct {

	// configuration
	MaxIt       int     // max iterations
	Ftol        float64 // tolerance on f({x})
	Gtol        float64 // convergence criterion for the zero gradient test
	Verbose     bool    // show messages
	History     bool    // save history
	UseBrent    bool    // use Brent method insted of LineSearch (Wolfe conditions)
	UseFRmethod bool    // use Fletcher-Reeves method instead of Polak-Ribiere
	CheckJfcn   bool    // check Jacobian function at all points during minimization

	// statistics and History (for debugging)
	NumFeval int      // number of calls to Ffcn (function evaluations)
	NumJeval int      // number of calls to Jfcn (Jacobian evaluations)
	NumIter  int      // number of iterations from last call to Solve
	Hist     *History // history of optimization data (for debugging)

	// internal
	size int       // problem dimension = len(x)
	tiny float64   // small number to rectify the special case of converging to exactly zero function value
	ffcn fun.Sv    // scalar function of vector: y = f({x})
	jfcn fun.Vv    // vector function of vector: g = dy/d{x} = deriv(f({x}), {x})
	u    la.Vector // direction vector for line minimization
	g    la.Vector // conjugate direction vector
	h    la.Vector // conjugate direction vector
	tmp  la.Vector // auxiliary vector

	// line solver
	lines *LineSearch     // line search
	lineb *num.LineSolver // line solver wrapping Brent's method
}

// NewConjGrad returns a new multidimensional optimizer using ConjGrad's method (no derivatives required)
//   size -- length(x)
//   ffcn -- scalar function of vector: y = f({x})
//   Jfcn -- Jacobian: g = dy/d{x} = deriv(f({x}), {x})
func NewConjGrad(size int, ffcn fun.Sv, Jfcn fun.Vv) (o *ConjGrad) {
	o = new(ConjGrad)
	o.size = size
	o.ffcn = ffcn
	o.jfcn = Jfcn
	o.MaxIt = 200
	o.Ftol = 1e-8
	o.Gtol = 1e-8
	o.tiny = 1e-18
	o.lines = NewLineSearch(size, ffcn, Jfcn)
	o.lineb = num.NewLineSolver(size, ffcn, Jfcn)
	o.u = la.NewVector(size)
	o.g = la.NewVector(size)
	o.h = la.NewVector(size)
	o.tmp = la.NewVector(size)
	return
}

// Min solves minimization problem
//
//  Input:
//    x -- [size] initial starting point (will be modified)
//
//  Output:
//    fmin -- f(x@min) minimum f({x}) found
//    x -- [modify input] position of minimum f({x})
//
func (o *ConjGrad) Min(x la.Vector) (fmin float64) {

	// line search function and counters
	linesearch := o.lines.Wolfe
	nfeval := &o.lines.NumFeval
	nJeval := &o.lines.NumJeval
	if o.UseBrent {
		linesearch = func(x, u la.Vector, dum1 bool, dum2 float64) (λ, fmin float64) { return o.lineb.MinUpdateX(x, u) }
		nfeval = &o.lineb.NumFeval
		nJeval = &o.lineb.NumJeval
	}

	// initializations
	fx := o.ffcn(x) // fx := f(x)
	o.jfcn(o.u, x)  // u := dy/dx
	for j := 0; j < o.size; j++ {
		o.u[j] = -o.u[j] // u := -dy/dx
		o.g[j] = o.u[j]  // g := -dy/dx
		o.h[j] = o.u[j]  // h := g
	}
	o.NumFeval = 1
	o.NumJeval = 1
	fmin = fx

	// history
	var λhist float64
	var uhist la.Vector
	if o.History {
		o.Hist = NewHistory(o.MaxIt, fmin, x, o.ffcn)
		uhist = la.NewVector(o.size)
	}

	// auxiliary
	var coef, temp, test, nume, deno, γ float64

	// estimate old f(x)
	fold := fx + o.u.Norm()/2.0 // TODO: find reference to this

	// iterations
	for o.NumIter = 0; o.NumIter < o.MaxIt; o.NumIter++ {

		// exit point # 1: old gradient is exactly zero
		deno = la.VecDot(o.g, o.g)
		if math.Abs(deno) < o.tiny {
			return
		}

		// line minimization
		λhist, fmin = linesearch(x, o.u, true, fold) // x := x @ min
		o.NumFeval += *nfeval
		o.NumJeval += *nJeval

		// update fold
		fold = fx

		// history
		if o.History {
			uhist.Apply(λhist, o.u)
			o.Hist.Append(fmin, x, uhist)
		}

		// exit point # 2: converged on f
		if 2.0*math.Abs(fmin-fx) <= o.Ftol*(math.Abs(fmin)+math.Abs(fx)+o.tiny) {
			return
		}

		// update fx and gradient dy/dx
		fx = fmin
		o.jfcn(o.u, x) // u := dy/dx
		o.NumJeval++

		// check Jacobian @ x
		if o.CheckJfcn {
			o.checkJacobian(x)
		}

		// test for convergence on zero gradient
		test = 0.0
		coef = utl.Max(fx, 1.0)
		for j := 0; j < o.size; j++ {
			temp = math.Abs(o.u[j]) * utl.Max(math.Abs(x[j]), 1.0) / coef
			if temp > test {
				test = temp
			}
		}

		// exit point # 3: converged on dy/dx (new)
		if test < o.Gtol {
			return
		}

		// compute scaling factor, noting that, now:
		//   u = -gNew
		//   g =  gOld
		if o.UseFRmethod {
			nume = la.VecDot(o.u, o.u) // nume := gNew ⋅ gNew  [Equation 10.8.5 page 517 of Ref 1]
		} else {
			la.VecAdd(o.tmp, 1, o.u, 1, o.g) // tmp := u + g = -gNew + gOld
			nume = la.VecDot(o.tmp, o.u)     // nume := (gOld - gNew) ⋅ (-gNew) = (gNew - gOld) ⋅ gNew  [Equation 10.8.7 page 517 of Ref 1]
			nume = utl.Max(nume, 0)          // avoid negative values
		}

		// update directions
		γ = nume / deno
		for j := 0; j < o.size; j++ {
			o.g[j] = -o.u[j]           // g := -dy/dx = gNew
			o.u[j] = o.g[j] + γ*o.h[j] // u := gNew + γ⋅hOld = hNew
			o.h[j] = o.u[j]            // h := hNew
		}
	}

	// did not converge
	chk.Panic("fail to converge after %d iterations\n", o.NumIter)
	return
}

// auxiliary ///////////////////////////////////////////////////////////////////////////////////////

// checkJacobian checks Jacobian at intermediate point x
func (o *ConjGrad) checkJacobian(x la.Vector) {
	tolJ := 1e-12
	for k := 0; k < o.size; k++ {
		dfdxk := num.DerivCen5(x[k], 1e-3, func(xk float64) float64 {
			copy(o.tmp, x)
			o.tmp[k] = xk
			o.NumFeval++
			return o.ffcn(o.tmp)
		})
		diff := math.Abs(o.u[k] - dfdxk)
		if diff > tolJ {
			chk.Panic("Jacobian function is incorrect. diff = %v\n", diff)
		}
	}
}