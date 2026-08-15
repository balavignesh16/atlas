package com.atlas.inventory.dto;

import jakarta.validation.constraints.NotNull;
import jakarta.validation.constraints.Positive;

public class ReleaseRequest {
    @NotNull(message = "Quantity must be provided")
    @Positive(message = "Quantity must be greater than 0")
    private Integer quantity;

    public ReleaseRequest() {}

    public ReleaseRequest(Integer quantity) {
        this.quantity = quantity;
    }

    public Integer getQuantity() {
        return quantity;
    }

    public void setQuantity(Integer quantity) {
        this.quantity = quantity;
    }
}
