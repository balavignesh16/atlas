package com.atlas.payment.controller;

import com.atlas.payment.domain.Payment;
import com.atlas.payment.domain.PaymentStatus;
import com.atlas.payment.dto.PaymentRequest;
import com.atlas.payment.exception.PaymentDeclinedException;
import com.atlas.payment.exception.PaymentNotFoundException;
import com.atlas.payment.repository.PaymentRepository;
import jakarta.validation.Valid;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;
import org.springframework.http.HttpStatus;
import org.springframework.http.ResponseEntity;
import org.springframework.web.bind.MissingRequestHeaderException;
import org.springframework.web.bind.annotation.*;

import java.time.Instant;
import java.util.UUID;

@RestController
@RequestMapping("/api/payments")
public class PaymentController {

    private static final Logger logger = LoggerFactory.getLogger(PaymentController.class);
    private final PaymentRepository paymentRepository;

    public PaymentController(PaymentRepository paymentRepository) {
        this.paymentRepository = paymentRepository;
    }

    @PostMapping
    public ResponseEntity<Payment> createPayment(
            @RequestHeader(value = "Idempotency-Key", required = true) String idempotencyKey,
            @Valid @RequestBody PaymentRequest request) {

        logger.info("Processing payment for order {} with idempotency key {}", request.orderId(), idempotencyKey);

        // Process idempotently
        Payment payment = paymentRepository.processIdempotentPayment(idempotencyKey, request, () -> {
            
            double amount = request.amount();
            
            // Deterministic Sandbox Failures
            if (Double.compare(amount, 8888.00) == 0) {
                logger.error("Sandbox Trigger: 8888.00 -> Simulating Server Error");
                throw new RuntimeException("Simulated payment server error");
            }
            
            if (Double.compare(amount, 9999.00) == 0) {
                logger.warn("Sandbox Trigger: 9999.00 -> Simulating Timeout");
                try {
                    Thread.sleep(6000); // Exceeds typical 2s timeout
                } catch (InterruptedException e) {
                    Thread.currentThread().interrupt();
                }
            }

            if (Double.compare(amount, 7777.00) == 0) {
                logger.warn("Sandbox Trigger: 7777.00 -> Simulating Decline");
                throw new PaymentDeclinedException("Payment declined by test simulator");
            }

            return new Payment(
                    "PAY-" + UUID.randomUUID().toString(),
                    request.orderId(),
                    request.amount(),
                    PaymentStatus.AUTHORIZED,
                    Instant.now(),
                    idempotencyKey
            );
        });

        return ResponseEntity.status(HttpStatus.CREATED).body(payment);
    }

    @GetMapping("/{paymentId}")
    public ResponseEntity<Payment> getPayment(@PathVariable String paymentId) {
        Payment payment = paymentRepository.getPaymentById(paymentId)
                .orElseThrow(() -> new PaymentNotFoundException("Payment not found: " + paymentId));
        return ResponseEntity.ok(payment);
    }
}
