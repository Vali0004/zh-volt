-- Define o protocolo
local olt_proto = Proto("olt", "Custom OLT Protocol")

-- Define os campos do cabeçalho com base na nova struct Packet
local f_magic   = ProtoField.bytes("olt.magic", "Magic", base.SPACE)
local f_request = ProtoField.uint16("olt.request", "Request ID", base.DEC)
local f_type    = ProtoField.uint16("olt.type", "Type", base.HEX)
local f_status  = ProtoField.uint8("olt.status", "Status", base.HEX)
local f_flag0   = ProtoField.uint8("olt.flag0", "Flag 0", base.HEX)
local f_flag1   = ProtoField.uint8("olt.flag1", "Flag 1", base.HEX)
local f_flag2   = ProtoField.uint8("olt.flag2", "Flag 2", base.HEX)
local f_data    = ProtoField.bytes("olt.data", "Data Payload", base.SPACE)

-- Registra os campos no protocolo
olt_proto.fields = { f_magic, f_request, f_type, f_status, f_flag0, f_flag1, f_flag2, f_data }

-- Função principal do Dissector
function olt_proto.dissector(buffer, pinfo, tree)
    local length = buffer:len()
    
    -- O cabeçalho mínimo tem 12 bytes (4 Magic + 2 RequestID + 2 Type + 1 Status + 3 Flags)
    if length < 12 then return end

    -- Verifica os bytes "OltMagic" (b9 58 d6 3a) para garantir que é o pacote correto
    local magic_tvb = buffer(0, 4)
    if magic_tvb:uint() ~= 0xb958d63a then
        return
    end

    -- Define o nome do protocolo na coluna principal do Wireshark
    pinfo.cols.protocol = "OLT"

    -- Cria a árvore principal no painel de detalhes do pacote
    local subtree = tree:add(olt_proto, buffer(), "OLT Protocol Data")

    -- Adiciona os campos à árvore com os offsets exatos do UnmarshalBinary
    subtree:add(f_magic, buffer(0, 4))
    subtree:add(f_request, buffer(4, 2))
    subtree:add(f_type, buffer(6, 2))
    subtree:add(f_status, buffer(8, 1))
    subtree:add(f_flag0, buffer(9, 1))
    subtree:add(f_flag1, buffer(10, 1))
    subtree:add(f_flag2, buffer(11, 1))

    -- O que sobrar depois do byte 11 (offset 12 em diante) é associado ao campo Data
    if length > 12 then
        subtree:add(f_data, buffer(12, length - 12))
    end
end

-- Associa este Dissector ao EtherType OLT (0x88b6)
local eth_table = DissectorTable.get("ethertype")
eth_table:add(0x88b6, olt_proto)