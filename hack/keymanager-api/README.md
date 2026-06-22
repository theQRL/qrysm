# Keymanager API

https://github.com/ethereum/keymanager-APIs

## Postman

You can use Postman to test the API. https://www.postman.com/

### Postman collection

In this package you will find the Postman collection for the keymanager API. 
You can import this collection in your own Postman instance to test the API.

#### Updating the collection

The collection will need to be exported and overwritten to update the collection. A PR should be created once the file
is updated.

#### Authentication

Our keymanager API requires a valid bearer token.

The validator persists this token at `<wallet-dir>/auth-token`. Read the token from that file and paste it into the authorization tab of each Postman request as the bearer token.
