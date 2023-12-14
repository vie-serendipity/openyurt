#!/bin/bash
set -e

echo "parameters are: " $*
 
function usage(){
    echo "$0 [Options]"
    echo -e "Options:"
    echo -e "\t-a, --arch\tCompile architecture, only supports amd64 and arm64 now"
    echo -e "\t-g, --goos\tCompile GOOS, only supports linux now"
    echo -e "\t-r, --regions\tCompile REGIONS, example cn-hangzhou,cn-beijing"
    echo -e "\t-v, --version\tCompile VERSION, example v1.3.0"
    echo -e "\t-h, --help\tHelp information"
    return 1
}
 
while [ $# -gt 0 ];do
    case $1 in
    --arch|-a)
      shift
      ARCH=$1
      shift
      if [[ ${ARCH} != "amd64" && ${ARCH} != "arm" && ${ARCH} != "arm64" ]]; then
         usage
      fi
      ;;
    --goos|-g)
      shift
      GOOS=$1
      shift
      if [[ ${GOOS} != "windows" && ${GOOS} != "linux" ]]; then
         usage
      fi
      ;;
    --regions|-r)
      shift
      REGIONLIST=$1
      shift
      ;;
    --version|-v)
      shift
      VERSION=$1
      shift
      ;;
    --ak)
      shift
      KUBERNETES_OSS_KEY=$1
      shift
      ;;
    --sk)
      shift
      KUBERNETES_OSS_SECRET=$1
      shift
      ;;
    --is-finance)
      shift
      IS_FINANCE=$1
      shift
      ;;
    --ak-finance)
      shift
      KUBERNETES_FINANCE_OSS_KEY=$1
      shift
      ;;
    --sk-finance)
      shift
      KUBERNETES_FINANCE_OSS_SECRET=$1
      shift
      ;;
    --all-regions)
      shift
      ALL_REGIONS=$1
      shift
      ;;
    --help|-h)
      shift
      usage
      ;;
    *)
      usage
      exit 1
      ;;
    esac
done
 
if [ "$VERSION" = "" ]; then
	echo "VERSION must be provided."; exit 1
fi
 
if [ "$REGIONLIST" = "" ]; then
	echo "REGIONLIST must be provided."; exit 1
fi
 
if [ "$ARCH" = "" ]; then
	echo "ARCH must be provided."; exit 1
fi
 
if [ "$GOOS" = "" ]; then
	echo "GOOS must be provided."; exit 1
fi
 
if [ "$KUBERNETES_OSS_KEY" = "" -o "$KUBERNETES_OSS_SECRET" = "" ];then
    echo "KUBERNETES_OSS_KEY & KUBERNETES_OSS_SECRET must be provided for rpm build." ; exit 1
fi
 
# ALL_REGIONS='cn-hangzhou cn-beijing cn-shanghai cn-qingdao cn-shenzhen cn-chengdu cn-zhangjiakou cn-huhehaote cn-heyuan cn-wulanchabu cn-guangzhou ap-southeast-3 ap-southeast-1 ap-southeast-2 ap-northeast-1 eu-central-1 me-east-1 us-east-1 us-west-1 ap-south-1 ap-southeast-5 eu-west-1 rus-west-1 cn-hongkong ap-northeast-2 ap-southeast-6 ap-southeast-7 cn-nanjing'
# ALL_FIN_REGIONS='cn-shanghai-finance-1 cn-shenzhen-finance-1 cn-beijing-finance-1'

REGIONS=(${REGIONLIST//./ })
if [ "$REGIONLIST" = "All" ]; then
	REGIONS="$ALL_REGIONS"
fi
 
function retry() {
    local n=0
    local try=$1
    local cmd="${@:2}"
    [[ $# -le 1 ]] && {
        echo "Usage $0 <retry_number> <Command>"
    }
    set +e
    until
        [[ $n -ge $try ]]; do
        $cmd && break || {
            echo "Command Fail.."
            ((n++))
            echo "retry $n :: [$cmd]"
            sleep 1
        }
    done
    set -e
    if [[ $n -ge $try ]]; then
        echo "retry failed with exceeded max retry count, $cmd"
        exit 2
    fi
}
 
function push_oss_bin(){
    bin="_output/local/bin/${GOOS}/${ARCH}/edge-hub"
    TARGET=$ARCH
 
    for reg in ${REGIONS[@]};
    do
      if [[ "$reg" == "cn-hangzhou-finance-1" ]] || [[ "$reg" == "cn-shanghai-mybk" ]] || [[ "$reg" == *"gov"* ]]; then
        echo "skip unsupported region $reg"
      elif [[ "$reg" == *"finance"* ]]; then
        echo "finance region $reg"
        retry 5 osscmd -i ${KUBERNETES_FINANCE_OSS_KEY} -k ${KUBERNETES_FINANCE_OSS_SECRET} -H oss-${reg}.aliyuncs.com put ${bin} oss://aliacs-k8s-${reg}/public/pkg/edgehub/${VERSION}/${TARGET}/
      else
        echo "normal region $reg"
        retry 5 osscmd -i ${KUBERNETES_OSS_KEY} -k ${KUBERNETES_OSS_SECRET} -H oss-${reg}.aliyuncs.com put ${bin} oss://aliacs-k8s-${reg}/public/pkg/edgehub/${VERSION}/${TARGET}/
      fi
    done
}

echo "regions are: $REGIONS"
push_oss_bin
