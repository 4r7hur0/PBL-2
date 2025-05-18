# PBL-2

## Para rodar a API

Para rodar a API é necessário navegar para o diretório da api, começando por 

`cd api/`

Depois que estiver no diretório da API, deve ser definido a variável de ambiente `ENTERPRISE_NAME` antes de rodar o comando. Assim, a API será instanciada com o nome decidido na variável de ambiente, o mesmo para a a porta que deve ser rodado, porém com o nome `ENTERPRISE_PORT`, e a quantidade de postos é definida pela variável `POSTS_QUANTITY`

Exemplos: 

`ENTERPRISE_NAME="SolAtlantico" ENTERPRISE_PORT=8081 POSTS_QUANTITY=2 go run .`
`ENTERPRISE_NAME="SertaoCarga" ENTERPRISE_PORT=8082 POSTS_QUANTITY=1 go run .`
`ENTERPRISE_NAME="CacauPower" ENTERPRISE_PORT=8083 POSTS_QUANTITY=1 go run .`

ou no Windows:  

$env:ENTERPRISE_NAME = "SolAtlantico"
$env:ENTERPRISE_PORT = "8081"
$env:POSTS_QUANTITY = "2"
go run .


$env:ENTERPRISE_NAME = "SertaoCarga"
$env:ENTERPRISE_PORT = "8082"
$env:POSTS_QUANTITY = "1"
go run .

$env:ENTERPRISE_NAME = "CacauPower"
$env:ENTERPRISE_PORT = "8083"
$env:POSTS_QUANTITY = "1"
go run .


Cada empresa deve ser instanciada em um terminal diferente