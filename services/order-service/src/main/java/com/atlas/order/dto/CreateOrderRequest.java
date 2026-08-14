package com.atlas.order.dto;

import jakarta.validation.constraints.NotBlank;
import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Positive;

public class CreateOrderRequest {

    @NotBlank(message = "productId must not be null or blank")
    private String productId;

    @NotNull(message = "quantity must not be null")
    @Positive(message = "quantity must be greater than 0")
    private Integer quantity;

    public CreateOrderRequest() {
    }

    public CreateOrderRequest(String productId, Integer quantity) {
        this.productId = productId;
        this.quantity = quantity;
    }

    public String getProductId() {
        return productId;
    }

    public void setProductId(String productId) {
        this.productId = productId;
    }

    public Integer getQuantity() {
        return quantity;
    }

    public void setQuantity(Integer quantity) {
        this.quantity = quantity;
    }
}
