package io.konveyor.demo.ordermanagement.service;

import io.konveyor.demo.ordermanagement.model.Customer;
import org.springframework.data.domain.Page;
import org.springframework.data.domain.Pageable;

public interface ICustomerService {
    public Customer findById(Long id); 
	
	public Page<Customer>findAll(Pageable pageable);
    
}
