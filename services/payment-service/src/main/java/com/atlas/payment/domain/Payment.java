package com.atlas.payment.domain;

import java.time.Instant;

public class Payment {
    private String paymentId;
    private String orderId;
    private double amount;
    private PaymentStatus status;
    private Instant createdAt;
    private String idempotencyKey;

    public Payment() {}

    public Payment(String paymentId, String orderId, double amount, PaymentStatus status, Instant createdAt, String idempotencyKey) {
        this.paymentId = paymentId;
        this.orderId = orderId;
        this.amount = amount;
        this.status = status;
        this.createdAt = createdAt;
        this.idempotencyKey = idempotencyKey;
    }

    public String getPaymentId() { return paymentId; }
    public void setPaymentId(String paymentId) { this.paymentId = paymentId; }

    public String getOrderId() { return orderId; }
    public void setOrderId(String orderId) { this.orderId = orderId; }

    public double getAmount() { return amount; }
    public void setAmount(double amount) { this.amount = amount; }

    public PaymentStatus getStatus() { return status; }
    public void setStatus(PaymentStatus status) { this.status = status; }

    public Instant getCreatedAt() { return createdAt; }
    public void setCreatedAt(Instant createdAt) { this.createdAt = createdAt; }

    public String getIdempotencyKey() { return idempotencyKey; }
    public void setIdempotencyKey(String idempotencyKey) { this.idempotencyKey = idempotencyKey; }
}
