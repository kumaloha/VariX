# Graph-First Foundation Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Introduce a first-class graph domain model and compile compatibility adapter without breaking the current CLI, compile, verify, or memory flows.

**Architecture:** Add a new `model` package as the future domain truth boundary, keep existing `compile` types as compatibility surfaces, and prove the bridge with adapter-focused tests before integrating downstream systems.

**Tech Stack:** Go, existing `compile` package types, Go test, gofmt

---

### Task 1: Add failing tests for the new graph domain model

**Files:**
- Create: `tests/model/content_graph_test.go`
- Create: `tests/model/compile_adapter_test.go`

- [ ] **Step 1: Write validation tests for required graph fields**
- [ ] **Step 2: Run `../tests/go-test.sh ./model` and verify failure because package/files do not exist yet**
- [ ] **Step 3: Write adapter tests covering legacy compile record -> model subgraph mapping**
- [ ] **Step 4: Re-run `../tests/go-test.sh ./model` and verify failure for missing implementation**

### Task 2: Implement the model package

**Files:**
- Create: `varix/model/node.go`
- Create: `varix/model/edge.go`
- Create: `varix/model/subgraph.go`
- Create: `varix/model/card.go`
- Create: `varix/model/verify.go`

- [ ] **Step 1: Add node / edge / subgraph / card / verify types**
- [ ] **Step 2: Add validation methods for node, edge, subgraph**
- [ ] **Step 3: Run `../tests/go-test.sh ./model` and fix compile/test failures until green**

### Task 3: Implement legacy compile compatibility adapter

**Files:**
- Create: `varix/model/compile_adapter.go`
- Modify: `tests/model/compile_adapter_test.go`

- [ ] **Step 1: Add compile record adapter and verification-status mapping helpers**
- [ ] **Step 2: Preserve compatibility by mapping legacy graph nodes/edges into the new domain model without changing existing compile behavior**
- [ ] **Step 3: Run `../tests/go-test.sh ./model` and confirm adapter tests pass**

### Task 4: Verify repo safety

**Files:**
- Modify: none expected beyond files above

- [ ] **Step 1: Run `gofmt -w varix/model/*.go`**
- [ ] **Step 2: Run `cd varix && ../tests/go-test.sh ./...`**
- [ ] **Step 3: Review diff to ensure no unrelated changes leaked in**

