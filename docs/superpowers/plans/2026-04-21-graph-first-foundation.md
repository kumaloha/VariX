# Graph-First Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a first-class graph domain model and compile compatibility adapter without breaking the current CLI, compile, verify, or memory flows.

**Architecture:** Add a new `graphmodel` package as the future domain truth boundary, keep existing `compile` types as compatibility surfaces, and prove the bridge with adapter-focused tests before integrating downstream systems.

**Tech Stack:** Go, existing `compile` package types, Go test, gofmt

---

### Task 1: Add failing tests for the new graph domain model

**Files:**
- Create: `varix/graphmodel/model_test.go`
- Create: `varix/graphmodel/compile_adapter_test.go`

- [ ] **Step 1: Write validation tests for required graph fields**
- [ ] **Step 2: Run `go test ./graphmodel` and verify failure because package/files do not exist yet**
- [ ] **Step 3: Write adapter tests covering legacy compile record -> graphmodel subgraph mapping**
- [ ] **Step 4: Re-run `go test ./graphmodel` and verify failure for missing implementation**

### Task 2: Implement the graphmodel package

**Files:**
- Create: `varix/graphmodel/node.go`
- Create: `varix/graphmodel/edge.go`
- Create: `varix/graphmodel/subgraph.go`
- Create: `varix/graphmodel/card.go`
- Create: `varix/graphmodel/verify.go`

- [ ] **Step 1: Add node / edge / subgraph / card / verify types**
- [ ] **Step 2: Add validation methods for node, edge, subgraph**
- [ ] **Step 3: Run `go test ./graphmodel` and fix compile/test failures until green**

### Task 3: Implement legacy compile compatibility adapter

**Files:**
- Create: `varix/graphmodel/compile_adapter.go`
- Modify: `varix/graphmodel/compile_adapter_test.go`

- [ ] **Step 1: Add compile record adapter and verification-status mapping helpers**
- [ ] **Step 2: Preserve compatibility by mapping legacy graph nodes/edges into the new domain model without changing existing compile behavior**
- [ ] **Step 3: Run `go test ./graphmodel` and confirm adapter tests pass**

### Task 4: Verify repo safety

**Files:**
- Modify: none expected beyond files above

- [ ] **Step 1: Run `gofmt -w varix/graphmodel/*.go`**
- [ ] **Step 2: Run `cd varix && go test ./...`**
- [ ] **Step 3: Review diff to ensure no unrelated changes leaked in**

