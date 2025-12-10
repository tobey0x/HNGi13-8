# Wallet Service API

A backend wallet service built with Go that allows users to deposit money using Paystack, manage wallet balances, view transaction history, and transfer funds to other users.

## 📚 Interactive API Documentation

**Swagger UI available at:** `http://localhost:8080/swagger/index.html`

## Prerequisites

- Go 1.21 or higher
- PostgreSQL database
- Paystack account (test or live)
- Google OAuth credentials

## Setup Instructions

### 1. Clone and Install Dependencies

```bash
cd ./HNGi13-8
go mod download
```

### 2. Database Setup

Create a PostgreSQL database:

```sql
CREATE DATABASE wallet_db;
```

### 3. Environment Configuration

Copy `.env.example` to `.env` and fill in your credentials:

```bash
cp .env.example .env
```

### 4. Run the Application

```bash
go run main.go
```

The server will start on `http://localhost:8080`

## API Endpoints

### Authentication

#### Google Sign-In
```
GET /auth/google
```
Redirects to Google OAuth consent screen.

#### Google Callback
```
GET /auth/google/callback
```
Handles OAuth callback and returns JWT token.

---

### API Key Management (Requires JWT)

#### Create API Key
```
POST /keys/create
Authorization: Bearer <jwt_token>

{
  "name": "wallet-service",
  "permissions": ["deposit", "transfer", "read"],
  "expiry": "1D"
}
```

**Valid expiry formats:** `1H` (hour), `1D` (day), `1M` (month), `1Y` (year)

**Response:**
```json
{
  "api_key": "sk_live_xxxxx",
  "expires_at": "2025-01-01T12:00:00Z"
}
```

#### Rollover Expired API Key
```
POST /keys/rollover
Authorization: Bearer <jwt_token>

{
  "expired_key_id": "uuid",
  "expiry": "1M"
}
```

#### List API Keys
```
GET /keys/list
Authorization: Bearer <jwt_token>
```

#### Revoke API Key
```
DELETE /keys/:id
Authorization: Bearer <jwt_token>
```

---

### Wallet Operations

#### Deposit Money (Initialize Paystack Transaction)

```bash
POST /wallet/deposit

# Option 1: Use JWT
Authorization: Bearer <jwt_token>

# Option 2: Use API Key (choose one, not both)
x-api-key: <api_key>

{
  "amount": 5000
}
```

**Amount is in kobo (smallest currency unit). 5000 kobo = ₦50**

**Response:**
```json
{
  "reference": "TXN_1234567890",
  "authorization_url": "https://checkout.paystack.com/..."
}
```

#### Paystack Webhook (Mandatory)
```
POST /wallet/paystack/webhook
x-paystack-signature: <signature>

{
  "event": "charge.success",
  "data": {
    "reference": "TXN_1234567890",
    "amount": 5000,
    "status": "success"
  }
}
```

This endpoint verifies the Paystack signature and credits the wallet.

#### Check Deposit Status

```bash
GET /wallet/deposit/:reference/status

# Option 1: Use JWT
Authorization: Bearer <jwt_token>

# Option 2: Use API Key (choose one, not both)
x-api-key: <api_key>
```

**Response:**
```json
{
  "reference": "TXN_1234567890",
  "status": "success",
  "amount": 5000
}
```

#### Get Wallet Balance

```bash
GET /wallet/balance

# Option 1: Use JWT (full access)
Authorization: Bearer <jwt_token>

# Option 2: Use API Key with "read" permission (choose one, not both)
x-api-key: <api_key>
```

**Response:**
```json
{
  "balance": 15000
}
```

#### Get Transaction History

```bash
GET /wallet/transactions

# Option 1: Use JWT (full access)
Authorization: Bearer <jwt_token>

# Option 2: Use API Key with "read" permission (choose one, not both)
x-api-key: <api_key>
```

**Response:**
```json
[
  {
    "type": "deposit",
    "amount": 5000,
    "status": "success"
  },
  {
    "type": "transfer",
    "amount": 3000,
    "status": "success"
  }
]
```

#### Transfer Funds

```bash
POST /wallet/transfer

# Option 1: Use JWT (full access)
Authorization: Bearer <jwt_token>

# Option 2: Use API Key with "transfer" permission (choose one, not both)
x-api-key: <api_key>

{
  "wallet_number": "4566678954356",
  "amount": 3000
}
```

**Response:**
```json
{
  "status": "success",
  "message": "Transfer completed"
}
```

---

## Authentication Methods

The API supports **two authentication methods** - you only need to provide **one** (not both):

### 1. JWT Token (Full Access)
```bash
Authorization: Bearer <jwt_token>
```
- ✅ Obtained after Google sign-in
- ✅ Has **full access** to all wallet operations
- ✅ No permission restrictions
- ✅ Used for end-user authentication

**Priority:** If both JWT and API key are provided, **JWT takes precedence** and API key is ignored.

### 2. API Key (Permission-Based Access)
```bash
x-api-key: sk_live_xxxxx
```
- ✅ Created via `/keys/create` endpoint (requires JWT)
- ✅ Requires specific permissions: `deposit`, `transfer`, `read`
- ✅ Maximum 5 active keys per user
- ✅ Can expire and be rolled over
- ✅ Can be revoked
- ✅ Used for service-to-service authentication (backend services, automated scripts)

**Note:** API key users are restricted to their assigned permissions only.

---

## API Key Permissions

- **deposit**: Allows initiating deposit transactions
- **transfer**: Allows transferring funds to other wallets
- **read**: Allows viewing balance and transaction history

---

## Testing with Paystack

For testing, use Paystack's test credentials and test cards:

**Test Card:**
- Card Number: `4084084084084081`
- CVV: `408`
- Expiry: Any future date
- PIN: `0000`
- OTP: `123456`

---

## Project Structure

```
.
├── config/          # Configuration management
├── database/        # Database connection and migrations
├── handlers/        # HTTP request handlers
│   ├── auth.go      # Google OAuth authentication
│   ├── apikeys.go   # API key management
│   └── wallet.go    # Wallet operations
├── middleware/      # Authentication and authorization
├── models/          # Database models
├── services/        # External services (Paystack)
├── utils/           # Helper functions
├── main.go          # Application entry point
├── go.mod           # Go module dependencies
└── .env             # Environment variables
```

---

## Error Handling

The API returns appropriate HTTP status codes:

- `200 OK`: Successful operation
- `201 Created`: Resource created successfully
- `400 Bad Request`: Invalid request data
- `401 Unauthorized`: Missing or invalid authentication
- `403 Forbidden`: Insufficient permissions
- `404 Not Found`: Resource not found
- `429 Too Many Requests`: Rate limit exceeded
- `500 Internal Server Error`: Server error

---

## Security Considerations

- JWT tokens are signed with a secret key
- Paystack webhooks are verified using HMAC-SHA512
- API keys are securely generated using crypto/rand
- Database transactions ensure atomicity
- Permission-based access control for API keys
- Rate limiting for API key requests

---

## Development

To run in development mode:

```bash
go run main.go
```

To build for production:

```bash
go build -o wallet-service
./wallet-service
```
