# Order Processing Example

Activities:

- `validate`
- `payment`
- `inventory`
- `email`
- `analytics`

The example is shaped as a DAG: payment and inventory can run after validation, email waits for both, and analytics can run independently after validation.

