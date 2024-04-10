# MAUCacheAdmin.Modify

#### update 2024-04-10
更新之前，删除所有的xml和cat，以及固定命名的如下文件。便于每次更新都获得最新文件
Lync Installer.pkg
MicrosoftTeams.pkg
Teams_osx.pkg
wdav-upgrade.pkg

#### update 2024-04-03
只提供PowerShell的实现

基于 https://github.com/pbowden-msft/MAUCacheAdmin 做的修改


解决/修正/新增如下 
- ManifestServer的格式变化，导致xml、cat的文件名变化，解决可能会存在需要下载带版本号的特定文件情况
- 追加Add-Type -AssemblyName System.Net.Http，这样在PowerShell5下依然可以正常执行
- 下载pkg的逻辑修改，小内存环境可以运行了，但是依然建议服务器至少配置8G内存
- -CachePath不存在，则自动创建，不再需要手动创建
- -ScratchPath不存在，则自动创建，不再需要手动创建
- 修改部分函数，确保支持特定参数

## 使用方法
准备一台Windows服务器，确保可以联网，下载本项目

将项目解压到本地 `c:\macupdate`下

执行powershell
```powershell
cd c:\macupdate
.\MacUpdatesOffice.Modify.ps1
```
或者可以手动指定参数
这里使用了一些参数
| 参数         | 默认值                      | 描述                                                         |
| ------------ | --------------------------- | ------------------------------------------------------------ |
| `workPath`   | `c:\MACupdate`              | 用户可以在运行脚本时指定工作路径。如果未指定，则使用此默认值。 |
| `maupath`    | `C:\inetpub\wwwroot\maunew6`| 用户可以在运行脚本时指定MAU存储离线文件的（Microsoft AutoUpdate）路径。如果未指定，则使用此默认值。 |
| `mautemppath`| `c:\MACupdate\temp`         | 用户可以在运行脚本时指定MAU临时路径。如果未指定，则使用此默认值。 |

下面是一个带参数的，可能的例子
```powershell
cd c:\macupdate
.\MacUpdatesOffice.Modify.ps1  -workPath "D:\NewWorkPath" -maupath "D:\NewMauPath" -mautemppath "D:\NewTempPath"

