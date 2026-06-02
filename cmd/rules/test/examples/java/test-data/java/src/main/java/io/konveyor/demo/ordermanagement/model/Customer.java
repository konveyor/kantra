package io.konveyor.demo.ordermanagement.model;

import javax.persistence.Column;
import javax.persistence.Entity;
import javax.persistence.GeneratedValue;
import javax.persistence.GenerationType;
import javax.persistence.Id;
import javax.persistence.SequenceGenerator;
import javax.persistence.Table;

@Entity
@Table(name = "customers")
public class Customer {
	@Id
    @SequenceGenerator(
            name = "customersSequence",
            sequenceName = "customers_id_seq",
            allocationSize = 1,
            initialValue = 6)
    @GeneratedValue(strategy = GenerationType.SEQUENCE, generator = "customersSequence")
	private Long id;
	
	@Column(length = 20)
	private String username;
	
	@Column(length = 20)
	private String name;
	
	@Column(length = 40)
	private String surname;
	
	@Column(length = 250)
	private String address;
	
	@Column(name = "zipcode", length = 10)
	private String zipCode;
	
	@Column(length = 40)
	private String city;
	
	@Column(length = 40)
	private String country;
	
	public Long getId() {
		return id;
	}
	public void setId(Long id) {
		this.id = id;
	}
	public String getUsername() {
		return username;
	}
	public void setUsername(String username) {
		this.username = username;
	}
	public String getName() {
		return name;
	}
	public void setName(String name) {
		this.name = name;
	}
	public String getSurname() {
		return surname;
	}
	public void setSurname(String surname) {
		this.surname = surname;
	}
	public String getAddress() {
		return address;
	}
	public void setAddress(String address) {
		this.address = address;
	}
	public String getZipCode() {
		return zipCode;
	}
	public void setZipCode(String zipCode) {
		this.zipCode = zipCode;
	}
	public String getCity() {
		return city;
	}
	public void setCity(String city) {
		this.city = city;
	}
	public String getCountry() {
		return country;
	}
	public void setCountry(String country) {
		this.country = country;
	}
	
	@Override
	public String toString() {
		return "Customer [id=" + id + ", username=" + username + ", name=" + name + ", surname=" + surname
				+ ", address=" + address + ", zipCode=" + zipCode + ", city=" + city + ", country=" + country + "]";
	}
}
