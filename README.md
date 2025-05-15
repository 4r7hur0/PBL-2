# PBL-2

## Para rodar a API

Para rodar a API é necessário navegar para o diretório da api, começando por 

`cd api/`

Depois que estiver no diretório da API, deve ser definido a variável de ambiente `ENTERPRISE_NAME` antes de rodar o comando. Assim, a API será instanciada com o nome decidido na variável de ambiente, o mesmo para a a porta que deve ser rodado, porém com o nome `ENTERPRISE_PORT`="


`ENTERPRISE_NAME="NomeDaEmpresa" ENTERPRISE_PORT=8081 go run .`