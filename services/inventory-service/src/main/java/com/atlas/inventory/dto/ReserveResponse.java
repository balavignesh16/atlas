package com.atlas.inventory.dto;

public class ReserveResponse {
    private String productId;
    private int reservedQuantity;
    private int remainingQuantity;

    public ReserveResponse() {}

    public ReserveResponse(String productId, int reservedQuantity, int remainingQuantity) {
        this.productId = productId;
        this.reservedQuantity = reservedQuantity;
        this.remainingQuantity = remainingQuantity;
    }

    public String getProductId() { return productId; }
    public void setProductId(String productId) { this.productId = productId; }

    public int getReservedQuantity() { return reservedQuantity; }
    public void setReservedQuantity(int reservedQuantity) { this.reservedQuantity = reservedQuantity; }

    public int getRemainingQuantity() { return remainingQuantity; }
    public void setRemainingQuantity(int remainingQuantity) { this.remainingQuantity = remainingQuantity; }
}
