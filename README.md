This is the Base58 website.

Currently a work in progress.

## Testing Stripe Hooks

Stripe has [great instructions](https://stripe.com/docs/payments/handling-payment-events) for how to test their hook callbacks. Here's a quick how-to though.

1. Install the `stripe` utility
2. Register with `./stripe login`. You can use your existing API key with `./stripe login --api-key <KEY>`
3. You'll need two terminal windows. In one, startup the console: 

	./stripe listen --forward-to http://localhost:8080/stripe-hook

4. In a 2nd window, send an event. You'll need to include some metadata to get things to work.

	./stripe trigger payment_intent.succeeded --add payment_intent:metadata['registration_id']=<reg_id>

You can get a registration ID from Notion by going to the class signups and opening one of them as a Page. The Page ID (listed in the URL) is the `reg_id`.
