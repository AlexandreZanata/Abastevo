-- EMP-ORG-004 review correction. The all-zero CNPJ computes valid check digits
-- but is not a registration. Every full CNPJ reaches storage through an
-- establishment, so this refuses it there; the root 00000000 itself remains a
-- real registration root and stays valid. Only the first condition differs.
CREATE OR REPLACE FUNCTION org_legal_cnpj_valid(value TEXT) RETURNS BOOLEAN
 LANGUAGE plpgsql IMMUTABLE PARALLEL SAFE SET search_path=pg_catalog AS $$
DECLARE ending INTEGER;i INTEGER;total INTEGER;digit INTEGER;weight INTEGER;remainder INTEGER;
BEGIN
 IF value IS NULL OR value !~ '^[A-Z0-9]{12}[0-9]{2}$' OR value='00000000000000' THEN RETURN false; END IF;
 FOR ending IN 12..13 LOOP
  total=0;weight=2;
  FOR i IN REVERSE ending..1 LOOP
   total=total+(ascii(substr(value,i,1))-48)*weight;weight=CASE WHEN weight=9 THEN 2 ELSE weight+1 END;
  END LOOP;
  remainder=total%11;digit=CASE WHEN remainder<2 THEN 0 ELSE 11-remainder END;
  IF ascii(substr(value,ending+1,1))-48<>digit THEN RETURN false; END IF;
 END LOOP;
 RETURN true;
END $$;

