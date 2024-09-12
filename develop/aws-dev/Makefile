.PHONY: terraform install certs

terraform:
	terraform apply -var-file pglass-s2c.tfvars

install:
	./scp.sh ./scripts /home/ec2-user/
	echo 'chmod +x ./scripts/*; ./scripts/install-bins.sh' | ./ssh.sh
	./scp-s2s-proxy-bin.sh
	./scp.sh ./certs /home/ec2-user/
	echo 'sudo rm -rf /etc/temporal/certs; sudo mv /home/ec2-user/certs/ /etc/temporal/' | ./ssh.sh
	echo './scripts/write-configs.sh' | ./ssh.sh

certs:
	rm -rf ./certs
	mkdir -p ./certs
	omni kubectl --context s-cgs-ue1-k get secrets -n temporal temporal-server-identity --template '{{index .data "ca.crt"}}' | base64 -d > ./certs/ca.crt
	omni kubectl --context s-cgs-ue1-k get secrets -n temporal temporal-server-identity --template '{{index .data "tls.crt"}}' | base64 -d > ./certs/tls.crt
	omni kubectl --context s-cgs-ue1-k get secrets -n temporal temporal-server-identity --template '{{index .data "tls.key"}}' | base64 -d > ./certs/tls.key
