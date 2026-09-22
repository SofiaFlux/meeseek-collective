package summa42

default decision := {"outcome": "DENY", "reason_codes": ["default_deny"]}

decision := {"outcome": "ALLOW", "reason_codes": ["low_risk"]} if {
  input.risk == "LOW"
  input.authority_valid == true
}
