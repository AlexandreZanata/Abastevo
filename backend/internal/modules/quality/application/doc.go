// Package application owns the FQ-04a assay publication contract: one
// validated manifest plus staged rows become exactly one published revision
// with an atomic current-pointer switch. Persistence lives in
// quality/adapters behind the Publisher port; enumeration, scoring and
// reads belong to FQ-05/FQ-12/FQ-08.
package application
