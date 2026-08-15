package com.atlas.order.client;

import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.http.HttpStatus;
import org.springframework.stereotype.Component;
import org.springframework.web.client.ResourceAccessException;
import org.springframework.web.client.RestClient;
import org.springframework.web.server.ResponseStatusException;

@Component
public class PaymentClient {

    private static final Logger logger = LoggerFactory.getLogger(PaymentClient.class);
    private final RestClient restClient;
    private final String paymentServiceUrl;

    public PaymentClient(RestClient.Builder restClientBuilder,
                         @Value("${payment.service.url:http://localhost:8086}") String paymentServiceUrl) {
        this.restClient = restClientBuilder.build();
        this.paymentServiceUrl = paymentServiceUrl;
    }

    public void authorizePayment(String orderId, double amount, String idempotencyKey, String correlationId) {
        String url = paymentServiceUrl + "/api/payments";
        PaymentRequest request = new PaymentRequest(orderId, amount);

        logger.info("Attempting payment authorization for orderId={}, amount={}", orderId, amount);

        try {
            restClient.post()
                    .uri(url)
                    .header("Idempotency-Key", idempotencyKey)
                    .header("X-Correlation-ID", correlationId)
                    .body(request)
                    .retrieve()
                    .onStatus(status -> status.is4xxClientError() || status.is5xxServerError(), (req, res) -> {
                        int statusCode = res.getStatusCode().value();
                        if (statusCode == 402) {
                            throw new ResponseStatusException(HttpStatus.PAYMENT_REQUIRED, "Payment was declined");
                        } else if (statusCode == 409) {
                            throw new ResponseStatusException(HttpStatus.CONFLICT, "Payment idempotency conflict");
                        } else if (statusCode >= 500) {
                            throw new ResponseStatusException(HttpStatus.BAD_GATEWAY, "Payment service error");
                        } else {
                            throw new ResponseStatusException(res.getStatusCode(), "Payment service client error");
                        }
                    })
                    .toBodilessEntity();
        } catch (ResourceAccessException e) {
            if (e.getMessage() != null && e.getMessage().toLowerCase().contains("timeout")) {
                throw new ResponseStatusException(HttpStatus.GATEWAY_TIMEOUT, "Payment service timeout", e);
            }
            throw new ResponseStatusException(HttpStatus.SERVICE_UNAVAILABLE, "Payment service unavailable", e);
        }
    }
    
    public record PaymentRequest(String orderId, double amount) {}
}
