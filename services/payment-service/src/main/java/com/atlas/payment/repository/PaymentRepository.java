package com.atlas.payment.repository;

import com.atlas.payment.domain.Payment;
import com.atlas.payment.dto.PaymentRequest;
import com.atlas.payment.exception.IdempotencyConflictException;
import org.springframework.stereotype.Repository;

import java.util.Optional;
import java.util.concurrent.ConcurrentHashMap;
import java.util.function.Supplier;

@Repository
public class PaymentRepository {

    // Maps idempotencyKey -> Payment
    private final ConcurrentHashMap<String, Payment> paymentsByIdempotencyKey = new ConcurrentHashMap<>();
    
    // Maps paymentId -> Payment
    private final ConcurrentHashMap<String, Payment> paymentsById = new ConcurrentHashMap<>();

    public Payment processIdempotentPayment(String idempotencyKey, PaymentRequest request, Supplier<Payment> paymentSupplier) {
        return paymentsByIdempotencyKey.compute(idempotencyKey, (key, existing) -> {
            if (existing != null) {
                // Check if request parameters match
                if (!existing.getOrderId().equals(request.orderId()) || 
                    Double.compare(existing.getAmount(), request.amount()) != 0) {
                    throw new IdempotencyConflictException("Idempotency key was already used with different request parameters");
                }
                return existing;
            }
            
            // Create new payment
            Payment newPayment = paymentSupplier.get();
            paymentsById.put(newPayment.getPaymentId(), newPayment);
            return newPayment;
        });
    }

    public Optional<Payment> getPaymentById(String paymentId) {
        return Optional.ofNullable(paymentsById.get(paymentId));
    }
    
    public void clear() {
        paymentsByIdempotencyKey.clear();
        paymentsById.clear();
    }
}
