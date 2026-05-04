# BDD Test Steps Architecture

This architecture organizes Behave steps into **3 clear layers**: Core, Support Services, and Feature Steps.

## 📐 Directory Structure

```
steps/
├── core/
│   ├── auth_steps.py
│   ├── assertion_steps.py
│   └── __init__.py
│
├── support/
│   ├── api_client.py
│   ├── common_utils.py
│   ├── services/
│   │   ├── domain_service.py
│   │   ├── workflow_service.py
│   │   └── __init__.py
│   └── __init__.py
│
└── features_steps/
    ├── domain_steps.py
    ├── domain_workflow.py (optional, only for complex multi-step workflows)
    ├── another_domain_steps.py
    ├── another_domain_workflow.py (optional, only for complex multi-step workflows)
    └── __init__.py
```

## 📚 Layer Descriptions

### Layer 1: `core/` - Foundation & Infrastructure

**Purpose**: Authentication and generic assertions.

**Files**:
- `auth_steps.py`: JWT token generation, role-based authentication headers
- `assertion_steps.py`: Generic HTTP response assertions (status codes, headers, etc.)

**Responsibilities**:
- JWT token creation and claims management
- Authentication headers for different roles
- Response status code validation
- Generic error response assertions

**Constraints**:
- ✅ Token generation, header creation
- ✅ Generic, reusable assertions
- ❌ No business logic
- ❌ No API calls (except auth endpoint if needed)

---

### Layer 2: `support/` - Services & Infrastructure

#### `api_client.py` - HTTP Client & URL Builder

**Purpose**: Low-level HTTP client and endpoint URL builders.

**Responsibilities**:
- URL construction for all endpoints (e.g., `template_create_url()`, `contract_approve_url()`)
- HTTP method wrappers (`post_json()`, `get_with_headers()`, `put_json()`, `delete()`)
- Default headers and timeout handling
- Request/response logging

**Constraints**:
- ✅ URL building
- ✅ HTTP protocol handling
- ✅ Standard headers setup
- ❌ No business logic
- ❌ No multi-step workflows

---

#### `common_utils.py` - Utility Functions

**Purpose**: Shared utility functions and helpers.

**Examples**:
- Type conversions (e.g., `category_to_template_type()`)
- Data formatting
- String manipulation
- Common calculations

---

#### `services/` - Domain Business Logic

**Purpose**: Domain-specific business logic and workflows.

**File Organization**:
- One service class per domain (e.g., `TemplateService`, `ContractService`)
- Naming convention: `{Domain}Service`

**Responsibilities**:
- Multi-step workflows (create → submit → approve)
- Automatic data generation
- Error handling with meaningful messages
- Role-based authentication headers
- State management and validation

**Key Method Patterns**:
- Single operations: `create(...)`, `submit(...)`, `approve(...)`
- Composite workflows: `create_and_approve(...)`, `full_lifecycle(...)`
- Queries: `fetch(...)`, `search(...)`

**Constraints**:
- ✅ Multi-step workflows
- ✅ Automatic data generation
- ✅ Error handling with meaningful messages
- ✅ Role-based authentication headers
- ❌ No Gherkin step definitions
- ❌ No feature-specific logic

---

### Layer 3: Feature Steps - Gherkin Step Definitions

**Purpose**: Gherkin step definitions that bridge features to services.

**File Organization**:
- One file per domain/feature area (e.g., `template_api_steps.py`, `contract_workflow_steps.py`)
- Naming convention: `{domain}_{operation}_steps.py`
  - `{domain}_api_steps.py`: Direct API operations
  - `{domain}_workflow_steps.py`: Multi-step workflows

**Responsibilities**:
- Map Gherkin steps (@given/@when/@then) to service method calls
- Store results in context for subsequent steps
- Call assertions from core layer
- Manage test state between steps

**Constraints**:
- ✅ Call services for business logic
- ✅ Store results in context
- ✅ Use core assertions
- ❌ No direct API calls
- ❌ No complex business logic
- ❌ No duplicate workflows

---

## 🔄 Data Flow

```
Feature File (Gherkin)
        ↓
    Step Definition (@when/@then/@given)
        ↓
    Service (TemplateService, ContractService)
        ↓
    API Client (HTTP wrapper)
        ↓
    Backend API
```
