package com.kestrelfreight.shipment.carrier;

import com.kestrelfreight.shipment.model.Carrier;
import org.springframework.data.jpa.repository.JpaRepository;

import java.util.List;

public interface CarrierRepository extends JpaRepository<Carrier, String> {

    List<Carrier> findByActiveTrueOrderByPriorityAscCodeAsc();
}
