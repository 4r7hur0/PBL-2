# PBL-2

## Para rodar a API

Antes de rodar a API, é necessário navegar para o diretório do registry, esse servidor é responsável por descobrir as URLs das APIs das cidades, se ele não for inicializado a comunicação entre cidades não funcionará:

`cd registry/registry-server`

Após isso, deve ser definida a porta do registry por meio da variável de ambiente `REGISTRY_PORT` e rodar o arquivo: 

$env:REGISTRY_PORT="9000" 

`go run .` 

Para rodar a API é necessário navegar para o diretório da api, começando por 

`cd api/`

Depois que estiver no diretório da API, deve ser definido a variável de ambiente `ENTERPRISE_NAME` antes de rodar o comando. Assim, a API será instanciada com o nome decidido na variável de ambiente, o mesmo para a a porta que deve ser rodado, porém com o nome `ENTERPRISE_PORT`, a quantidade de postos é definida pela variável `POSTS_QUANTITY`e a cidade por qual aquela empresa é responsável pela variável `OWNED_CITY`

Exemplos: 

`ENTERPRISE_NAME="SolAtlantico" ENTERPRISE_PORT=8081 POSTS_QUANTITY=2 go run .`
`ENTERPRISE_NAME="SertaoCarga" ENTERPRISE_PORT=8082 POSTS_QUANTITY=1 go run .`
`ENTERPRISE_NAME="CacauPower" ENTERPRISE_PORT=8083 POSTS_QUANTITY=1 go run .`

ou no Windows:  

$env:ENTERPRISE_NAME = "SolAtlantico"
$env:ENTERPRISE_PORT = "8081"
$env:OWNED_CITY = "Salvador"
$env:POSTS_QUANTITY = "2"
$env:REGISTRY_URL="http://registry:9000"
go run .


$env:ENTERPRISE_NAME=CacauPower
$env:ENTERPRISE_PORT=8083
$env:OWNED_CITY=Ilheus
$env:POSTS_QUANTITY=1
$env:REGISTRY_URL="http://registry:9000"

go run .


Cada empresa deve ser instanciada em um terminal diferente